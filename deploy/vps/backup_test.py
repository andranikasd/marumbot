"""Backup configuration and failure regressions; no Docker or network required."""
import fcntl
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

SCRIPT = Path(__file__).with_name("backup-offsite.sh").resolve()


class BackupTest(unittest.TestCase):
    def run_backup(self, *, container="abc123", journal_present=True, upload_fails=False, lock_held=False):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            journal = root / "custom journal"
            if journal_present:
                journal.mkdir()
            fake = root / "bin"
            fake.mkdir()
            calls = root / "calls"
            docker = fake / "docker"
            docker.write_text("""#!/usr/bin/env python3
import os, sys
args = sys.argv[1:]
if args[0] == 'inspect':
    print(os.environ['TEST_JOURNAL'])
elif 'ps' in args:
    print(os.environ['TEST_CONTAINER'])
elif 'pg_dump' in args:
    print('test archive')
elif 'pg_restore' in args:
    sys.stdin.read()
else:
    sys.exit(2)
""")
            restic = fake / "restic"
            restic.write_text("""#!/usr/bin/env python3
import json, os, sys
with open(os.environ['TEST_CALLS'], 'a') as stream:
    stream.write(json.dumps(sys.argv[1:]) + '\\n')
if sys.argv[1] == 'backup' and os.environ['TEST_UPLOAD_FAILS'] == '1':
    sys.exit(1)
""")
            docker.chmod(0o755)
            restic.chmod(0o755)
            env = os.environ.copy()
            env.update({
                "PATH": str(fake) + os.pathsep + env["PATH"],
                "RESTIC_REPOSITORY": "test-repository",
                "RESTIC_PASSWORD_FILE": str(root / "password"),
                "MARUM_COMPOSE_ENV": str(root / "compose.env"),
                "MARUM_BACKUP_DIR": str(root / "backups"),
                "MARUM_BACKUP_LOCK": str(root / "backup.lock"),
                # A stale service variable must not override the actual mount.
                "MARUM_ERASURE_JOURNAL_DIR": str(root / "wrong journal"),
                "TEST_JOURNAL": str(journal),
                "TEST_CONTAINER": container,
                "TEST_CALLS": str(calls),
                "TEST_UPLOAD_FAILS": "1" if upload_fails else "0",
            })
            with (root / "backup.lock").open("a") as lock:
                if lock_held:
                    fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
                result = subprocess.run(["sh", str(SCRIPT)], env=env, capture_output=True, text=True)
                if lock_held:
                    self.assertIn("another offsite backup is running", result.stderr)
                    self.assertFalse((root / "backups").exists())
                else:
                    # Success and upload failure must both release the lock.
                    fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            invocations = [json.loads(line) for line in calls.read_text().splitlines()] if calls.exists() else []
            return result.returncode, invocations, str(journal)

    def test_backs_up_actual_mount_and_then_prunes(self):
        code, calls, journal = self.run_backup()
        self.assertEqual(code, 0)
        self.assertEqual([call[0] for call in calls], ["backup", "forget"])
        self.assertEqual(calls[0][-1], journal)

    def test_missing_or_multiple_containers_fail_without_upload(self):
        for container in ("", "abc123\ndef456"):
            code, calls, _ = self.run_backup(container=container)
            self.assertNotEqual(code, 0)
            self.assertEqual(calls, [])

    def test_missing_journal_fails_without_upload(self):
        code, calls, _ = self.run_backup(journal_present=False)
        self.assertNotEqual(code, 0)
        self.assertEqual(calls, [])

    def test_failed_upload_does_not_prune(self):
        code, calls, _ = self.run_backup(upload_fails=True)
        self.assertNotEqual(code, 0)
        self.assertEqual([call[0] for call in calls], ["backup"])

    def test_overlapping_backup_fails_before_dump_or_upload(self):
        code, calls, _ = self.run_backup(lock_held=True)
        self.assertNotEqual(code, 0)
        self.assertEqual(calls, [])


if __name__ == "__main__":
    unittest.main()
