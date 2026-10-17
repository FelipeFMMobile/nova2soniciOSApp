"""Account setup must preserve users, secrets and dry-run semantics."""
import argparse
import contextlib
import io
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch, MagicMock

sys.path.insert(0, str(Path(__file__).resolve().parents[2] / 'scripts'))
import dev_account
import dev


class AccountDevTests(unittest.TestCase):
    def test_old_cli_response_is_not_reported_as_invalid_mfa(self):
        settings = dict(STS_COGNITO_REGION='us-east-1', STS_COGNITO_USER_POOL_ID='pool',
                        STS_COGNITO_CLIENT_ID='client')
        with patch.object(dev_account, 'aws_json', side_effect=[
                {'UserPool': {}}, {'MfaConfiguration': 'ON'}, {'UserPoolClient': {}}]):
            with self.assertRaisesRegex(RuntimeError, 'AWS CLI não retornou UserPoolTier'):
                dev_account.preflight(settings, 'default', [])

    def test_project_cli_is_used_without_changing_global_path(self):
        with tempfile.TemporaryDirectory() as directory:
            local = Path(directory) / 'data/account/aws-runtime/bin/aws'
            local.parent.mkdir(parents=True)
            local.touch()
            with patch.object(dev_account, 'ROOT', Path(directory)), \
                 patch.object(dev_account.subprocess, 'run', return_value=MagicMock(returncode=0, stdout='{}')) as run:
                dev_account.aws_json('default', 'us-east-1', 'cognito-idp', 'get-user-pool-mfa-config')
                self.assertEqual(run.call_args.args[0][0], str(local))

    def supervise(self, failure):
        args = argparse.Namespace(agent_port=18081, gateway_port=18080)
        settings = {key: 'synthetic-' + key for key in dev_account.SECRET_KEYS}
        settings['STS_IDENTITY_PROVIDER'] = 'fake'
        children = [MagicMock() for _ in range(3)]
        for child in children:
            child.poll.return_value = None
        launch = children
        ready = None
        if failure == 'launch':
            launch = [children[0], OSError('launch failed')]
        elif failure == 'readiness':
            ready = [None, RuntimeError('not ready')]
        elif failure == 'exit':
            children[1].poll.return_value = 1
        output = io.StringIO()
        with tempfile.TemporaryDirectory() as directory, contextlib.ExitStack() as stack:
            stack.enter_context(patch.object(dev_account, 'ROOT', Path(directory)))
            stack.enter_context(patch.object(dev_account, 'wait_http'))
            stack.enter_context(patch.object(dev_account, 'seed'))
            stack.enter_context(patch.object(dev_account, 'service_key', side_effect=['account-key', 'personal-key']))
            stack.enter_context(patch.object(dev, 'ensure_ports'))
            checked = stack.enter_context(patch.object(dev, 'checked'))
            stack.enter_context(patch.object(dev, 'wait_ready', side_effect=ready))
            stopped = stack.enter_context(patch.object(dev, 'stop_children'))
            popen = stack.enter_context(patch.object(dev_account.subprocess, 'Popen', side_effect=launch))
            stack.enter_context(patch.object(dev_account.time, 'sleep', side_effect=KeyboardInterrupt))
            stack.enter_context(contextlib.redirect_stdout(output))
            with self.assertRaises((RuntimeError, OSError, KeyboardInterrupt)):
                dev_account.run(args, settings, 'unused', [], False, False, 'private-test-token', '', '')
            expected = 1 if failure == 'launch' else 2 if failure == 'readiness' else 3
            self.assertEqual(stopped.call_args.args[0], children[:expected])
            for call in popen.call_args_list:
                self.assertTrue(call.kwargs['stdout'].closed)
                self.assertTrue(call.kwargs['start_new_session'])
            # Existing development databases stay available for the next run.
            self.assertFalse(any('down' in call.args[0] for call in checked.call_args_list))
            for path in (Path(directory) / 'logs/dev').glob('*.log'):
                self.assertEqual(path.stat().st_mode & 0o777, 0o600)
        self.assertNotIn('private-test-token', output.getvalue())
        return popen.call_args_list

    def test_partial_launch_failure_stops_started_children_and_closes_logs(self):
        self.supervise('launch')

    def test_readiness_failure_stops_both_started_agents(self):
        self.supervise('readiness')

    def test_runtime_exit_stops_remaining_services(self):
        self.supervise('exit')

    def test_interrupt_stops_all_services_and_isolates_credentials(self):
        calls = self.supervise('interrupt')
        personal, account, gateway = [call.kwargs['env'] for call in calls]
        self.assertEqual(account['STS_LITELLM_API_KEY'], 'account-key')
        self.assertEqual(personal['STS_LITELLM_API_KEY'], 'personal-key')
        self.assertNotIn('STS_LITELLM_API_KEY', gateway)
        self.assertNotIn('STS_MCP_CONTEXT_SECRET', gateway)
        self.assertNotIn('STS_IDENTITY_SERVICE_TOKEN', account)

    def test_fake_dry_run_has_no_side_effects(self):
        args = argparse.Namespace(dry_run=True)
        with patch.object(dev_account, 'load_settings', return_value={}), \
             patch('builtins.input', side_effect=['1', '']), \
             patch.object(dev_account, 'save_settings') as save, \
             patch.object(dev_account, 'run') as run, \
             patch.object(dev_account, 'aws_json') as aws, \
             contextlib.redirect_stdout(io.StringIO()):
            self.assertEqual(dev_account.interactive(args, nova=False), 0)
        save.assert_not_called()
        run.assert_not_called()
        aws.assert_not_called()

    def test_provider_switch_preserves_existing_environment(self):
        with patch.object(dev_account, 'load_settings', return_value={'STS_IDENTITY_PROVIDER': 'cognito'}), \
             patch.object(dev_account, 'save_settings') as save:
            with self.assertRaises(RuntimeError):
                dev_account.interactive(argparse.Namespace(dry_run=False), nova=False)
        save.assert_not_called()

    def test_settings_are_private_and_reject_line_injection(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / '.env'
            dev_account.save_settings({'STS_SERVICE_TOKEN': 'test-only'}, path)
            self.assertEqual(path.stat().st_mode & 0o777, 0o600)
            self.assertEqual(dev_account.load_settings(path), {'STS_SERVICE_TOKEN': 'test-only'})
            with self.assertRaises(RuntimeError):
                dev_account.save_settings({'STS_SERVICE_TOKEN': 'value\nOTHER=value'}, path)
            self.assertEqual(dev_account.load_settings(path), {'STS_SERVICE_TOKEN': 'test-only'})

    def test_existing_untracked_cognito_user_is_never_adopted(self):
        settings = {'STS_IDENTITY_PROVIDER': 'cognito', 'STS_COGNITO_REGION': 'region',
                    'STS_COGNITO_USER_POOL_ID': 'pool', 'STS_FIXTURE_NAMESPACE': 'test'}
        with tempfile.TemporaryDirectory() as directory, \
             patch.object(dev_account, 'FIXTURES', Path(directory) / 'fixtures.json'), \
             patch.object(dev_account, 'aws_json', return_value={'UserAttributes': []}) as aws, \
             patch.object(dev_account, 'json_request') as request:
            with self.assertRaises(RuntimeError):
                dev_account.seed(settings, 'default', ['fixture@example.invalid'], 'identity', 'profile')
        self.assertEqual(aws.call_count, 1)
        self.assertEqual(aws.call_args.args[3], 'admin-get-user')
        request.assert_not_called()

    def test_fake_seed_uses_only_internal_fixture_apis(self):
        settings = {'STS_IDENTITY_PROVIDER': 'fake', 'STS_IDENTITY_SERVICE_TOKEN': 'identity-key',
                    'STS_PROFILE_SERVICE_TOKEN': 'profile-key'}
        with tempfile.TemporaryDirectory() as directory, \
             patch.object(dev_account, 'FIXTURES', Path(directory) / 'fixtures.json'), \
             patch.object(dev_account, 'aws_json') as aws, \
             patch.object(dev_account, 'json_request', return_value={'client_code': 'a' * 64}) as request, \
             contextlib.redirect_stdout(io.StringIO()):
            dev_account.seed(settings, 'default', [], 'identity', 'profile')
        aws.assert_not_called()
        self.assertEqual(request.call_count, 6)
        self.assertTrue(all(call.args[1] == '/internal/fixtures' for call in request.call_args_list))

    def test_account_key_restricts_tools_and_requires_policy(self):
        tools = {'sts-identity': ['identity.begin_login']}
        with patch.object(dev_account, 'json_request', return_value={'key': 'test-key'}) as request:
            self.assertEqual(dev_account.service_key('base', 'master', 'sts-account', tools), 'test-key')
        body = request.call_args.args[3]
        self.assertTrue(body['metadata']['account_policy_required'])
        self.assertEqual(body['object_permission']['mcp_tool_permissions'], tools)
        self.assertNotIn('master', str(body))


if __name__ == '__main__':
    unittest.main()
