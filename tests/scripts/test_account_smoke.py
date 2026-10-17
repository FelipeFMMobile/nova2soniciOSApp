import copy
import contextlib
import io
import os
import subprocess
import tempfile
from pathlib import Path
import sys
import unittest
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parents[2] / 'scripts'))
import account_smoke


class AccountSmokeTests(unittest.TestCase):
    def test_privacy_audit_accepts_content_free_metrics(self):
        account_smoke.assert_private(b'{"duration_ms":12,"failed":false}', ['synthetic-private'])

    def test_privacy_audit_does_not_repeat_sensitive_match(self):
        with self.assertRaises(RuntimeError) as failure:
            account_smoke.assert_private(b'private-canary-value', ['private-canary-value'])
        self.assertNotIn('private-canary-value', str(failure.exception))

    def test_empty_spend_does_not_claim_realtime_audited(self):
        output = io.StringIO()
        with tempfile.TemporaryDirectory() as directory, \
             patch.object(account_smoke.subprocess, 'run', return_value=subprocess.CompletedProcess([], 0, b'', b'')), \
             contextlib.redirect_stdout(output):
            account_smoke.audit_privacy(['docker', 'compose'], {}, Path(directory), ['private-value'])
        self.assertIn('spend rows inspected: 0', output.getvalue())
        self.assertIn('Realtime spend audit remains pending', output.getvalue())

    def test_audit_failure_does_not_print_database_or_service_error(self):
        output = io.StringIO()
        with tempfile.TemporaryDirectory() as directory, \
             patch.object(account_smoke.subprocess, 'run', return_value=subprocess.CompletedProcess([], 1, b'private-value', b'private-value')), \
             contextlib.redirect_stdout(output), self.assertRaises(RuntimeError) as failure:
            account_smoke.audit_privacy(['docker', 'compose'], {}, Path(directory), ['private-value'])
        self.assertNotIn('private-value', output.getvalue() + str(failure.exception))

    def test_aws_and_existing_runtime_credentials_are_not_inherited(self):
        with patch.dict(os.environ, {'AWS_ACCESS_KEY_ID': 'private', 'AWS_PROFILE': 'production',
                                    'STS_AGENT_TOKEN': 'private', 'NOVA_SECRET': 'private', 'PATH': '/test'}, clear=True):
            self.assertEqual(account_smoke.clean_environment(), {'PATH': '/test'})

    def test_xctestrun_v1_injects_only_test_runner_environment(self):
        document = {'NovaVoiceUITests': {'TestBundlePath': '__TESTROOT__/NovaVoiceUITests.xctest',
                    'EnvironmentVariables': {'EXISTING': 'preserved'}, 'TestHostPath': 'host'},
                    '__xctestrun_metadata__': {'FormatVersion': 1}}
        values = {'STS_TEST_URL': 'ws://127.0.0.1:12345/v1/voice', 'STS_TEST_TOKEN': 'test-only'}
        account_smoke.xctest_environment(document, values)
        self.assertEqual(document['NovaVoiceUITests']['EnvironmentVariables'], {'EXISTING': 'preserved', **values})
        self.assertEqual(document['__xctestrun_metadata__'], {'FormatVersion': 1})

    def test_xctestrun_v2_preserves_other_targets(self):
        other = {'BlueprintName': 'Unrelated', 'EnvironmentVariables': {'EXISTING': 'preserved'}}
        document = {'TestConfigurations': [{'TestTargets': [copy.deepcopy(other), {'BlueprintName': 'NovaVoiceUITests'}]}]}
        account_smoke.xctest_environment(document, {'STS_ACCOUNT_UI': '1'})
        targets = document['TestConfigurations'][0]['TestTargets']
        self.assertEqual(targets[0], other)
        self.assertEqual(targets[1]['EnvironmentVariables'], {'STS_ACCOUNT_UI': '1'})

    def test_missing_ui_target_fails_before_execution(self):
        with self.assertRaises(RuntimeError):
            account_smoke.xctest_environment({'Other': {'BlueprintName': 'Other'}}, {'STS_ACCOUNT_UI': '1'})


if __name__ == '__main__':
    unittest.main()
