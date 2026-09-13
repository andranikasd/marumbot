import contextlib
import importlib.util
import io
import json
from pathlib import Path
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('recover_admin', Path(__file__).with_name('recover-admin.py'))
recovery = importlib.util.module_from_spec(spec)
spec.loader.exec_module(recovery)

class RecoveryTests(unittest.TestCase):
    def exercise(self, confirmation='new recovery passphrase', enabled=True):
        calls=[]
        hashed='pbkdf2-sha256:210000:c2FsdHNhbHRzYWx0c2FsdA:aGFzaGhhc2hoYXNoaGFzaA'
        def command(args, **kwargs):
            calls.append((args, kwargs.get('input')))
            if len(calls)==1:return json.dumps({'username':'admin','enabled':enabled})
            if '-hash-password' in args:return hashed
            return json.dumps({'username':'admin','enrolled':True})
        error=None
        output=io.StringIO()
        with patch.object(recovery.os,'geteuid',return_value=0), patch.object(recovery,'command',command), \
             patch.object(recovery.getpass,'getpass',side_effect=['new recovery passphrase',confirmation]), \
             contextlib.redirect_stdout(output):
            try:recovery.recover()
            except RuntimeError as exc:error=exc
        self.assertNotIn(hashed,output.getvalue())
        self.assertNotIn('new recovery passphrase',output.getvalue())
        return calls,error,output.getvalue()

    def test_reset_sends_secrets_only_over_stdin_and_preserves_totp(self):
        calls,error,output=self.exercise()
        self.assertIsNone(error)
        self.assertEqual(len(calls),3)
        self.assertNotIn('new recovery passphrase',repr([args for args,_ in calls]))
        self.assertIn('--single-transaction',calls[-1][0])
        self.assertIn('version = version + 1',calls[-1][1])
        self.assertNotIn('SET totp_secret',calls[-1][1])
        self.assertIn('INSERT INTO admin_audit',calls[-1][1])
        self.assertIn('fresh authenticator',output)

    def test_confirmation_mismatch_does_not_change_account(self):
        calls,error,_=self.exercise(confirmation='different passphrase')
        self.assertIsNotNone(error)
        self.assertEqual(len(calls),1)

    def test_disabled_admin_is_not_reenabled(self):
        calls,error,_=self.exercise(enabled=False)
        self.assertIsNotNone(error)
        self.assertEqual(len(calls),1)
