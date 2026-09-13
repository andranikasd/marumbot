"""Exercise deployment failure boundaries without touching Docker or production."""
import contextlib
import base64
import hashlib
import importlib.util
import io
import json
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

BASE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location('deploy_latest', BASE / 'deploy-latest.py')
deploy = importlib.util.module_from_spec(spec)
spec.loader.exec_module(deploy)


class DeployTests(unittest.TestCase):
    def test_version_update_preserves_quoted_secrets_and_overrides_images(self):
        source = "MARUM_BOT_TOKEN='private$token'\nMARUM_VERSION=old\nexport MARUM_IMAGE='old'\n"
        result = deploy.release_env(source, 'a' * 40)
        self.assertIn("MARUM_BOT_TOKEN='private$token'", result)
        self.assertNotIn('old', result)
        self.assertEqual(result.count('MARUM_IMAGE='), 1)
        self.assertIn('marum-vps:sha-' + 'a' * 40, result)

    def exercise(self, failure=None, dirty=False):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            repo, records, env = root / 'repo', root / 'records', root / 'compose.env'
            (repo / '.git').mkdir(parents=True)
            (repo / 'deploy/vps').mkdir(parents=True)
            (repo / 'queries/provisioning').mkdir(parents=True)
            for file in ['deploy/vps/init-db.sh', 'queries/provisioning/vps-role.sql']:
                (repo / file).touch()
            original = "MARUM_VERSION='old'\nMARUM_IDENTITY_KEY='preserve-me'\n"
            env.write_text(original)
            calls = []
            candidate = root / 'candidate'
            candidate.mkdir()
            (candidate / 'queries/provisioning').mkdir(parents=True)
            for filename in ['vps-role.sql', 'vps-grants.sql']:
                (candidate / 'queries/provisioning' / filename).write_text(filename)

            def run(args, **kwargs):
                calls.append(args)
                if failure and failure in args:
                    raise RuntimeError('injected failure')
                out = ''
                if 'status' in args:
                    out = ' M changed\n' if dirty else ''
                elif 'FETCH_HEAD^{commit}' in args:
                    out = 'a' * 40
                elif 'rev-parse' in args:
                    out = 'b' * 40
                elif 'archive' in args:
                    out = b'archive'
                elif 'pg_dump' in args:
                    kwargs['stdout'].write(b'verified dump')
                return subprocess.CompletedProcess(args, 0, out)

            error = None
            with patch.object(deploy, 'REPO', repo), patch.object(deploy, 'ENV', env), \
                 patch.object(deploy, 'NGINX_SITE', root / 'absent-nginx'), \
                 patch.object(deploy, 'RECORDS', records), patch.object(deploy, 'run', run), \
                 patch.object(deploy.tempfile, 'TemporaryDirectory', return_value=contextlib.nullcontext(str(candidate))), \
                 patch.object(deploy.urllib.request, 'urlopen', return_value=io.BytesIO(json.dumps(
                     {'status': 'ok', 'version': 'sha-' + 'a' * 12}).encode())), \
                 contextlib.redirect_stdout(io.StringIO()):
                try:
                    deploy.deploy()
                except RuntimeError as exc:
                    error = exc
            jobs = list(records.glob('deploy-*')) if records.exists() else []
            return calls, error, env.read_text(), original, [p.name for j in jobs for p in j.iterdir()]

    def test_success_fetches_remote_head_backs_up_before_migrating(self):
        calls, error, env, _, files = self.exercise()
        self.assertIsNone(error)
        self.assertTrue(any(c[-4:] == ['fetch', '--no-tags', 'origin', 'HEAD'] for c in calls))
        index = lambda word: next(i for i, c in enumerate(calls) if word in c)
        self.assertLess(index('build'), index('stop'))
        self.assertLess(index('pg_dump'), index('pg_restore'))
        self.assertLess(index('pg_restore'), index('run'))
        self.assertLess(index('run'), index('checkout'))
        self.assertIn('preserve-me', env)
        self.assertIn('success', files)
        self.assertFalse(any('down' in c or 'prune' in c for c in calls))
        self.assertTrue(any('--no-deps' in c and c[-1] == 'marum' for c in calls))

    def test_build_failure_leaves_running_app_and_env(self):
        calls, error, env, original, files = self.exercise(failure='build')
        self.assertIsNotNone(error)
        self.assertEqual(env, original)
        self.assertFalse(any('stop' in c for c in calls))
        self.assertNotIn('success', files)

    def test_migration_failure_keeps_backup_and_does_not_start_app(self):
        calls, error, env, original, files = self.exercise(failure='run')
        self.assertIsNotNone(error)
        self.assertEqual(env, original)
        self.assertIn('database.dump', files)
        self.assertNotIn('success', files)
        self.assertFalse(any('checkout' in c or '--force-recreate' in c for c in calls))

    def test_dirty_checkout_is_not_overwritten(self):
        calls, error, env, original, _ = self.exercise(dirty=True)
        self.assertIsNotNone(error)
        self.assertEqual(env, original)
        self.assertFalse(any('fetch' in c or 'stop' in c for c in calls))

    def test_invalid_backup_stops_before_migrations(self):
        calls, error, env, original, files = self.exercise(failure='pg_restore')
        self.assertIsNotNone(error)
        self.assertEqual(env, original)
        self.assertNotIn('database.dump', files)
        self.assertFalse(any('run' in c for c in calls))

    def test_unhealthy_release_never_records_success(self):
        _, error, env, _, files = self.exercise(failure='--force-recreate')
        self.assertIsNotNone(error)
        self.assertIn('sha-' + 'a' * 40, env)
        self.assertIn('previous.env', files)
        self.assertNotIn('success', files)

    def test_admin_diagnostic_checks_password_without_disclosing_hash(self):
        password = 'local diagnostic password'
        salt = b'0123456789abcdef'
        encode = lambda raw: base64.b64encode(raw).decode().rstrip('=')
        hashed = 'pbkdf2-sha256:210000:' + encode(salt) + ':' + encode(
            hashlib.pbkdf2_hmac('sha256', password.encode(), salt, 210000))
        configuration = {'services': {'marum': {'environment': {
            'MARUM_ADMIN_USER': 'admin', 'MARUM_ADMIN_PASSWORD_HASH': hashed}}}}
        for entered, expected in [(password, True), ('wrong password', False)]:
            with self.subTest(expected=expected), tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp)
                (root / 'queries/provisioning').mkdir(parents=True)
                (root / 'queries/provisioning/admin-login-check.sql').write_text('diagnostic SQL')
                output = io.StringIO()
                responses = [subprocess.CompletedProcess([], 0, json.dumps(configuration)),
                             subprocess.CompletedProcess([], 0, json.dumps({
                                 'enabled': True, 'enrolled': False, 'password_hash': hashed})),
                             subprocess.CompletedProcess([], 0, '2026-09-13T00:00:00Z')]
                with patch.object(deploy, 'REPO', root), patch.object(deploy, 'run', side_effect=responses), \
                     patch.object(deploy.getpass, 'getpass', return_value=entered), \
                     contextlib.redirect_stdout(output):
                    deploy.admin_check()
                self.assertIn(f'Password matches stored account: {expected}', output.getvalue())
                self.assertNotIn(hashed, output.getvalue())
                self.assertNotIn(entered, output.getvalue())

    def test_nginx_repair_preserves_certificate_and_restores_on_failure(self):
        for fail in [False, True]:
            with self.subTest(fail=fail), tempfile.TemporaryDirectory() as tmp:
                root=Path(tmp)
                site=root/'nginx.conf'
                original='ssl_certificate /etc/letsencrypt/live/example/fullchain.pem;\nadd_header Referrer-Policy no-referrer always;\n'
                site.write_text(original)
                calls=[]
                def run(args, **kwargs):
                    calls.append(args)
                    if fail and len(calls)==1:raise RuntimeError('invalid nginx config')
                with patch.object(deploy,'NGINX_SITE',site), patch.object(deploy,'run',run), contextlib.redirect_stdout(io.StringIO()):
                    if fail:
                        with self.assertRaises(RuntimeError):deploy.repair_nginx(root)
                    else:deploy.repair_nginx(root)
                self.assertEqual((root/'previous-nginx.conf').read_text(),original)
                self.assertEqual(site.read_text(),original if fail else original.replace('no-referrer','strict-origin'))
                self.assertEqual(calls[-1],['systemctl','reload','nginx'])


if __name__ == '__main__':
    unittest.main()
