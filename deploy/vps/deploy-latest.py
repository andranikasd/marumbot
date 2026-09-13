#!/usr/bin/env python3
"""Deploy origin HEAD on the existing Ubuntu VPS; secrets stay outside Git."""
import argparse
import base64
import fcntl
import getpass
import hashlib
import hmac
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile
import urllib.request

REPO = Path('/opt/marum')
ENV = Path('/etc/marum/compose.env')
RECORDS = Path('/var/lib/marum/release-records')


def run(args, **kwargs):
    # Never include command output in exceptions: config/SQL can contain secrets.
    result = subprocess.run(args, **kwargs)
    if result.returncode:
        raise RuntimeError(f'{args[0]} failed (exit {result.returncode})')
    return result


def compose(root=None, env=None):
    root, env = root or REPO, env or ENV
    args = ['docker', 'compose', '--project-name', 'marum-production',
            '--env-file', str(env), '-f', str(root / 'deploy/vps/compose.yml')]
    override = ENV.parent / 'proxy.override.yml'
    if override.exists():
        args += ['-f', str(override)]
    return args


def release_env(source, commit):
    values = {'MARUM_VERSION': f'sha-{commit[:12]}',
              'MARUM_IMAGE': f'marum-vps:sha-{commit}',
              'MARUM_MIGRATE_IMAGE': f'marum-migrate:sha-{commit}'}
    lines = [line for line in source.splitlines()
             if not any(re.match(r'^\s*(?:export\s+)?' + key + r'\s*=', line)
                        for key in values)]
    return '\n'.join(lines + [f"{key}='{value}'" for key, value in values.items()]) + '\n'


def private_write(path, content):
    with path.open('x') as out:
        os.chmod(path, 0o600)
        out.write(content)


def admin_check():
    dc = compose()
    config = json.loads(run(dc + ['config', '--format', 'json'],
                            capture_output=True, text=True).stdout)
    env = config['services']['marum']['environment']
    username = env.get('MARUM_ADMIN_USER', 'admin')
    print(f'Configured admin username: {username}')
    if not env.get('MARUM_ADMIN_PASSWORD_HASH'):
        raise RuntimeError('Admin is disabled in compose.env.')
    query = (REPO / 'queries/provisioning/admin-login-check.sql').read_text()
    raw = run(dc + ['exec', '-T', 'postgres', 'psql', '-XAt', '-U', 'marum_owner',
                    '-d', 'marum', '-v', 'ON_ERROR_STOP=1', '-v', 'username=' + username],
              input=query, capture_output=True, text=True).stdout.strip()
    if not raw:
        raise RuntimeError('Configured admin identity is absent. Check application startup/bootstrap.')
    identity = json.loads(raw)
    print('Account enabled:', identity['enabled'])
    print('Authenticator enrolled:', identity['enrolled'])
    print('Stored password matches bootstrap hash:',
          hmac.compare_digest(identity['password_hash'], env['MARUM_ADMIN_PASSWORD_HASH']))
    password = getpass.getpass('Admin password to check locally (hidden): ')
    try:
        kind, rounds, salt, expected = identity['password_hash'].split(':')
        if kind != 'pbkdf2-sha256' or not 1000 <= int(rounds) <= 1000000:
            raise ValueError()
        decode = lambda value: base64.b64decode(value + '=' * (-len(value) % 4), validate=True)
        salt, expected = decode(salt), decode(expected)
        if len(salt) < 16 or len(expected) != 32:
            raise ValueError()
        matched = hmac.compare_digest(hashlib.pbkdf2_hmac('sha256', password.encode(), salt,
                                                        int(rounds), 32), expected)
    except (ValueError, TypeError):
        raise RuntimeError('Stored password hash is malformed.') from None
    print('Password matches stored account:', matched)
    print('Server UTC:', run(['date', '-u', '+%Y-%m-%dT%H:%M:%SZ'],
                             capture_output=True, text=True).stdout.strip())
    print('First login: leave authenticator code blank, then enroll. Existing enrollment: use a fresh code.')
    print('No account, password, authenticator or session was changed.')


def deploy():
    if not (REPO / '.git').exists() or not ENV.is_file():
        raise RuntimeError('Requires the existing /opt/marum and /etc/marum/compose.env bootstrap.')
    # Refuse local content changes; earlier chmod repairs alone are harmless.
    dirty = run(['git', '-C', str(REPO), '-c', 'core.filemode=false', 'status',
                 '--porcelain', '--untracked-files=normal'], capture_output=True, text=True).stdout
    if dirty:
        raise RuntimeError('/opt/marum has local changes. Preserve/reconcile them before deploying.')
    run(['git', '-C', str(REPO), 'fetch', '--no-tags', 'origin', 'HEAD'])
    commit = run(['git', '-C', str(REPO), 'rev-parse', 'FETCH_HEAD^{commit}'],
                 capture_output=True, text=True).stdout.strip()
    if not re.fullmatch('[0-9a-f]{40}', commit):
        raise RuntimeError('Expected a full Git commit.')
    old_commit = run(['git', '-C', str(REPO), 'rev-parse', 'HEAD'],
                     capture_output=True, text=True).stdout.strip()
    print(f'Deploying origin HEAD: {commit}', flush=True)
    RECORDS.mkdir(parents=True, exist_ok=True, mode=0o700)
    job = Path(tempfile.mkdtemp(prefix='deploy-', dir=RECORDS))
    private_write(job / 'previous.env', ENV.read_text())
    private_write(job / 'previous-commit.txt', old_commit + '\n')
    private_write(job / 'target-commit.txt', commit + '\n')
    candidate_env = job / 'candidate.env'
    private_write(candidate_env, release_env(ENV.read_text(), commit))
    # Build only committed files; no untracked secrets can enter this context.
    with tempfile.TemporaryDirectory(prefix='marum-build-', dir='/opt') as temporary:
        candidate = Path(temporary)
        archive = run(['git', '-C', str(REPO), 'archive', commit], capture_output=True).stdout
        run(['tar', '-x', '-C', str(candidate)], input=archive)
        dc = compose(candidate, candidate_env)
        run(dc + ['config', '--quiet'])
        print('Building before interrupting the running application.', flush=True)
        run(dc + ['build', '--pull', 'marum', 'migrate'])
        current = compose()
        # An upgrade never replaces PostgreSQL or its volume.
        run(current + ['exec', '-T', 'postgres', 'pg_isready', '-U', 'marum_owner', '-d', 'marum'])
        stopped = False
        try:
            run(current + ['stop', 'marum'])
            stopped = True
            print(f'Saving pre-migration backup in {job}', flush=True)
            with (job / 'database.dump.partial').open('xb') as out:
                run(current + ['exec', '-T', 'postgres', 'pg_dump', '-U', 'marum_owner',
                               '-d', 'marum', '--format=custom'], stdout=out)
            with (job / 'database.dump.partial').open('rb') as source:
                run(current + ['exec', '-T', 'postgres', 'pg_restore', '--list'],
                    stdin=source, stdout=subprocess.DEVNULL)
            (job / 'database.dump.partial').rename(job / 'database.dump')
            # Reapply provisioning as owner, including default grants before migrations.
            run(current + ['exec', '-T', 'postgres', 'psql', '-X', '-U', 'marum_owner',
                           '-d', 'marum', '-v', 'ON_ERROR_STOP=1', '--single-transaction'],
                input=(candidate / 'queries/provisioning/vps-role.sql').read_bytes())
            run(dc + ['run', '--rm', '--no-deps', '-T', 'migrate', 'up'])
            run(current + ['exec', '-T', 'postgres', 'psql', '-X', '-U', 'marum_owner',
                           '-d', 'marum', '-v', 'ON_ERROR_STOP=1', '--single-transaction'],
                input=(candidate / 'queries/provisioning/vps-grants.sql').read_bytes())
            # Checkout with readable nonsecret files for the postgres bind mounts.
            previous_umask = os.umask(0o022)
            try:
                run(['git', '-C', str(REPO), 'checkout', '--detach', commit])
            finally:
                os.umask(previous_umask)
            for filename in ['deploy/vps/init-db.sh', 'queries/provisioning/vps-role.sql']:
                (REPO / filename).chmod(0o644)
            replacement = ENV.with_name('compose.env.next')
            private_write(replacement, candidate_env.read_text())
            os.replace(replacement, ENV)
            run(compose() + ['up', '-d', '--no-build', '--no-deps', '--force-recreate',
                             '--wait', '--wait-timeout', '240', 'marum'])
            with urllib.request.urlopen('http://127.0.0.1:8080/readyz', timeout=10) as response:
                ready = json.load(response)
            if ready.get('status') != 'ok' or ready.get('version') != f'sha-{commit[:12]}':
                raise RuntimeError('Readiness did not confirm the target release.')
            private_write(job / 'readiness.json', json.dumps(ready) + '\n')
            private_write(job / 'success', commit + '\n')
            print(f'Deployed and healthy: {commit}\nRelease record and backup: {job}')
        except BaseException:
            if stopped:
                print(f'Deployment interrupted after app stop. State preserved in {job}. '
                      'Inspect logs; no automatic schema rollback or volume deletion was attempted.', flush=True)
            raise
    print('Refresh Telegram keyboard with /start. Open its new inline dashboard button.')
    print('Admin login: use your configured username/password; first login enrolls an authenticator.')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--check-admin', action='store_true', help='Read-only password/enrollment diagnostic')
    args = parser.parse_args()
    if os.geteuid() != 0:
        parser.error('Run with sudo python3 deploy/vps/deploy-latest.py')
    os.umask(0o077)
    with open('/run/lock/marum-deploy.lock', 'w') as lock:
        try:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            parser.error('Another deployment or admin diagnostic is running.')
        try:
            admin_check() if args.check_admin else deploy()
        except (RuntimeError, OSError, ValueError) as error:
            # RuntimeError messages are ours; other exceptions might include input data.
            parser.exit(1, f'ERROR: {error if isinstance(error, RuntimeError) else type(error).__name__}\n')


if __name__ == '__main__':
    main()
