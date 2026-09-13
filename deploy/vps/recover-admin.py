#!/usr/bin/env python3
"""Set a new bootstrap admin password using authorized VPS root access."""
import fcntl
import getpass
import json
import os
from pathlib import Path
import re
import subprocess
import sys


def command(args, **kwargs):
    result = subprocess.run(args, capture_output=True, text=True, **kwargs)
    if result.returncode:
        raise RuntimeError('Recovery command failed. No credentials were printed; inspect service health.')
    return result.stdout.strip()


def recover():
    if os.geteuid() != 0:
        raise RuntimeError('Run with sudo python3 deploy/vps/recover-admin.py')
    dc = ['/usr/local/sbin/marum-compose']
    provisioning = Path(__file__).resolve().parents[2] / 'queries/provisioning'
    # Display only account status, never a password hash or TOTP secret.
    current = command(dc + ['exec', '-T', 'postgres', 'psql', '-XAt', '-U', 'marum_owner',
                            '-d', 'marum', '-v', 'ON_ERROR_STOP=1'], input=
                      (provisioning / 'admin-recover-account.sql').read_text())
    if not current:
        raise RuntimeError('No bootstrap admin account exists. No changes made.')
    account = json.loads(current)
    if not account['enabled']:
        raise RuntimeError('Bootstrap admin is disabled. Diagnose that state separately; no changes made.')
    print('Admin username:', account['username'])
    password = getpass.getpass('New admin passphrase (at least 16 characters): ')
    if len(password) < 16 or '\n' in password or '\r' in password:
        raise RuntimeError('Use at least 16 characters on one line. No changes made.')
    if password != getpass.getpass('Repeat new passphrase: '):
        raise RuntimeError('Passphrases do not match. No changes made.')
    hashed = command(dc + ['exec', '-T', 'marum', '/marum', '-hash-password'], input=password + '\n')
    if not re.fullmatch(r'pbkdf2-sha256:[0-9]+:[A-Za-z0-9+/]+:[A-Za-z0-9+/]+', hashed):
        raise RuntimeError('Unexpected hash format. No changes made.')
    sql = (provisioning / 'admin-recover-password.sql').read_text()
    updated = command(dc + ['exec', '-T', 'postgres', 'psql', '-XAt', '-U', 'marum_owner',
                            '-d', 'marum', '-v', 'ON_ERROR_STOP=1', '--single-transaction'],
                      input="\\set new_hash '" + hashed + "'\n" + sql)
    if not updated:
        raise RuntimeError('No enabled bootstrap account was updated.')
    account = json.loads(updated)
    print('Password reset for:', account['username'])
    print('Use a fresh authenticator code.' if account['enrolled'] else
          'Leave the authenticator code blank on first login, then enroll.')
    print('Existing sessions are invalidated by the account version change. Audit record saved.')


if __name__ == '__main__':
    try:
        with open('/run/lock/marum-deploy.lock', 'w') as lock:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            recover()
    except (RuntimeError, OSError, ValueError) as error:
        sys.exit(str(error) if isinstance(error, RuntimeError) else
                 'Recovery failed; no credentials were printed. Check service health.')
