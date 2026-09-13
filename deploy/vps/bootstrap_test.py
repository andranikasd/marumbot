"""Exercise bootstrap configuration without installing packages or launching Docker."""
import contextlib
import io
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile
import unittest
from unittest.mock import patch


BASE = Path(__file__).resolve().parent
CONFIG_CODE = re.findall(r"python3 - <<'PY'\n(.*?)\nPY", (BASE / "bootstrap-ubuntu.sh").read_text(), re.S)[0]


class BootstrapConfigTests(unittest.TestCase):
    def exercise(self, *, webhook=False, takeover=False, existing=False, build_fails=False, nginx=False, admin_password='long test admin passphrase'):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            env = root / 'compose.env'
            template = root / 'template'
            template.write_text((BASE / '.env.example').read_text())
            terminal = root / 'tty'
            terminal.write_text('marum.example.com\n' + ('admin.example.com\n' if nginx else '') + 'operator@example.com\n')
            if existing:
                env.write_text('original-key-must-survive')
            code = CONFIG_CODE.replace('/etc/marum/compose.env', str(env)).replace(
                '/opt/marum/deploy/vps/.env.example', str(template)).replace('/dev/tty', str(terminal))
            operations = []
            deleted = False

            def request(url, data, timeout):
                nonlocal deleted
                method = url.rsplit('/', 1)[1]
                operations.append(method)
                if method == 'deleteWebhook':
                    self.assertEqual(data, b'drop_pending_updates=false')
                    deleted = True
                result = {'url': 'https://old.example.com' if webhook and not deleted else ''}
                return io.BytesIO(json.dumps({'ok': True, 'result': result}).encode())

            def run(args, **kwargs):
                operations.append(args)
                if build_fails and 'build' in args:
                    raise subprocess.CalledProcessError(1, args)
                if '-hash-password' in args:
                    self.assertEqual(kwargs['input'], 'long test admin passphrase\n')
                    return subprocess.CompletedProcess(args, 0, '$pbkdf2$test$hash\n', '')
                return subprocess.CompletedProcess(args, 0)

            output = io.StringIO()
            error = None
            namespace = {}
            with patch.dict(os.environ, {'MARUM_SETUP_VERSION': 'sha-test', 'MARUM_SETUP_TAKE_OVER': str(int(takeover)), 'MARUM_SETUP_NGINX': str(int(nginx)), 'MARUM_SETUP_CLOUDFLARE': '0'}), \
                 patch('getpass.getpass', side_effect=['12345:synthetic_token', admin_password, admin_password]), \
                 patch('socket.getaddrinfo', return_value=[]), \
                 patch('urllib.request.urlopen', side_effect=request), \
                 patch('subprocess.run', side_effect=run), \
                 contextlib.redirect_stdout(output):
                try:
                    exec(compile(code, 'bootstrap config', 'exec'), namespace)
                except (SystemExit, subprocess.CalledProcessError) as exc:
                    error = exc
                finally:
                    if 'tty' in namespace:
                        namespace['tty'].close()
            contents = env.read_text() if env.exists() else ''
            mode = env.stat().st_mode & 0o777 if env.exists() else None
            self.assertNotIn('synthetic_token', output.getvalue())
            self.assertNotIn('long test admin passphrase', output.getvalue())
            self.assertNotIn('synthetic_token', repr(operations))
            return error, operations, contents, mode

    def test_fresh_config_is_private_and_preserves_hash_dollars(self):
        error, _, contents, mode = self.exercise()
        self.assertIsNone(error)
        self.assertEqual(mode, 0o600)
        self.assertIn("MARUM_ADMIN_PASSWORD_HASH='$pbkdf2$test$hash'", contents)
        values = dict(line.split('=', 1) for line in contents.splitlines() if '=' in line and not line.startswith('#'))
        self.assertNotEqual(values['POSTGRES_PASSWORD'], values['MARUM_DB_PASSWORD'])

    def test_existing_config_is_never_overwritten(self):
        error, operations, contents, _ = self.exercise(existing=True)
        self.assertIsInstance(error, SystemExit)
        self.assertEqual(operations, [])
        self.assertEqual(contents, 'original-key-must-survive')

    def test_webhook_without_takeover_stops_before_configuration(self):
        error, operations, contents, _ = self.exercise(webhook=True)
        self.assertIsInstance(error, SystemExit)
        self.assertNotIn('deleteWebhook', operations)
        self.assertEqual(contents, '')

    def test_takeover_preserves_updates_and_waits_for_successful_build(self):
        error, operations, _, _ = self.exercise(webhook=True, takeover=True)
        self.assertIsNone(error)
        build_index = next(i for i, op in enumerate(operations) if isinstance(op, list) and 'build' in op)
        self.assertGreater(operations.index('deleteWebhook'), build_index)

    def test_failed_build_keeps_keys_and_does_not_take_over_bot(self):
        error, operations, contents, mode = self.exercise(webhook=True, takeover=True, build_fails=True)
        self.assertIsInstance(error, subprocess.CalledProcessError)
        self.assertNotIn('deleteWebhook', operations)
        self.assertIn('MARUM_IDENTITY_KEY=', contents)
        self.assertEqual(mode, 0o600)

    def test_public_admin_requires_password(self):
        error, _, contents, _ = self.exercise(nginx=True, admin_password='')
        self.assertIsInstance(error, SystemExit)
        self.assertEqual(contents, '')

    def test_nginx_uses_override_and_stores_separate_admin_hostname(self):
        error, operations, contents, _ = self.exercise(nginx=True)
        self.assertIsNone(error)
        self.assertIn("MARUM_ADMIN_DOMAIN='admin.example.com'", contents)
        commands = [op for op in operations if isinstance(op, list)]
        self.assertTrue(all('/etc/marum/proxy.override.yml' in op for op in commands))
        self.assertFalse(any('caddy' in op for op in commands))


class CloudflareNginxTests(unittest.TestCase):
    def exercise(self, conflict=False):
        code = re.findall(r"python3 - <<'PY'\n(.*?)\nPY", (BASE / 'bootstrap-ubuntu.sh').read_text(), re.S)[1]
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / 'sites-available').mkdir()
            (root / 'sites-enabled').mkdir()
            (root / 'env').write_text("MARUM_DOMAIN='marum.example.com'\nMARUM_ADMIN_DOMAIN='admin.example.com'\nACME_EMAIL='operator@example.com'\n")
            (root / 'tty').write_text('a' * 32 + '\n8.8.8.8\n')
            for original, replacement in {
                '/etc/marum/compose.env': str(root / 'env'),
                '/dev/tty': str(root / 'tty'),
                '/etc/letsencrypt/cloudflare.ini': str(root / 'credentials'),
                '/etc/nginx/sites-available/marum': str(root / 'sites-available/marum'),
                '/etc/nginx/sites-enabled/marum': str(root / 'sites-enabled/marum'),
            }.items():
                code = code.replace(original, replacement)
            operations = []

            def request(req, timeout):
                operations.append((req.method, req.full_url, req.data))
                if '/settings/ssl' in req.full_url:
                    result = {'value': 'strict'}
                elif '/dns_records?' in req.full_url:
                    result = [{'type': 'CNAME', 'content': 'old.example.com'}] if conflict else []
                elif req.method == 'POST':
                    result = {'id': 'created'}
                else:
                    result = {'name': 'example.com'}
                return io.BytesIO(json.dumps({'success': True, 'result': result}).encode())

            output = io.StringIO()
            error = None
            with patch.dict(os.environ, {'MARUM_SETUP_CLOUDFLARE': '1'}), \
                 patch('getpass.getpass', return_value='synthetic_cloudflare_token'), \
                 patch('urllib.request.urlopen', side_effect=request), \
                 patch('subprocess.run') as run, contextlib.redirect_stdout(output):
                try:
                    exec(compile(code, 'nginx stage', 'exec'), {})
                except SystemExit as exc:
                    error = exc
            self.assertNotIn('synthetic_cloudflare_token', output.getvalue())
            self.assertNotIn('synthetic_cloudflare_token', repr(run.call_args_list))
            if conflict:
                self.assertIsInstance(error, SystemExit)
                self.assertFalse(any(op[0] != 'GET' for op in operations))
                self.assertFalse((root / 'credentials').exists())
                return
            self.assertIsNone(error)
            self.assertEqual((root / 'credentials').stat().st_mode & 0o777, 0o600)
            config = (root / 'sites-available/marum').read_text()
            self.assertIn('proxy_pass http://127.0.0.1:8080;', config)
            self.assertIn('proxy_pass http://127.0.0.1:8081;', config)
            self.assertIn('server_name admin.example.com;', config)
            self.assertEqual(config.count('location /app/'), 1)
            self.assertEqual(config.count('location / { return 404; }'), 1)
            cert = next(call.args[0] for call in run.call_args_list if call.args[0][0] == 'certbot')
            self.assertIn('dns-cloudflare', cert)
            self.assertIn('admin.example.com', cert)
            self.assertIn('marum.example.com', cert)
            writes = [json.loads(op[2]) for op in operations if op[0] == 'POST']
            self.assertEqual(len(writes), 2)
            self.assertTrue(all(item['proxied'] for item in writes))

    def test_two_hostnames_dns01_and_private_credentials(self):
        self.exercise()

    def test_existing_dns_target_is_not_overwritten(self):
        self.exercise(conflict=True)


class ResumeGuardsTests(unittest.TestCase):
    def test_resume_rejects_existing_config_and_journal(self):
        script = (BASE / 'bootstrap-ubuntu.sh').read_text()
        guard = script[script.index('if (( resume_before_config )); then'):script.index('# The checked-in resource envelope')]
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / 'repo/.git').mkdir(parents=True)
            (root / 'settings').mkdir()
            (root / 'journal').mkdir()
            guard = guard.replace('/opt/marum', str(root / 'repo')).replace('/etc/marum', str(root / 'settings')).replace(
                '/var/lib/marum/erasures', str(root / 'journal')).replace('/usr/local/sbin/marum-compose', str(root / 'wrapper'))
            prefix = 'set -eu\nresume_before_config=1\ndie() { echo "$*" >&2; exit 1; }\ngit() { :; }\n'
            def run():
                return subprocess.run(['bash'], input=prefix + guard, text=True, capture_output=True)
            self.assertEqual(run().returncode, 0)
            (root / 'settings/compose.env').write_text('existing-secret')
            result = run()
            self.assertNotEqual(result.returncode, 0)
            self.assertIn('Configuration already exists', result.stderr)
            self.assertEqual((root / 'settings/compose.env').read_text(), 'existing-secret')
            (root / 'settings/compose.env').unlink()
            (root / 'journal/deletion').write_text('preserve')
            result = run()
            self.assertNotEqual(result.returncode, 0)
            self.assertIn('Journal is not empty', result.stderr)
            self.assertEqual((root / 'journal/deletion').read_text(), 'preserve')


if __name__ == '__main__':
    unittest.main()
