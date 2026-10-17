import json
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import MagicMock

sys.path.insert(0, str(Path(__file__).resolve().parents[2] / 'scripts'))
import provision_account_aws as provisioner


class ProvisionAccountTests(unittest.TestCase):
    def session(self, account='selected'):
        session = MagicMock()
        sts, ses, cognito = MagicMock(), MagicMock(), MagicMock()
        sts.get_caller_identity.return_value = {'Account': account}
        ses.get_email_identity.return_value = {'VerifiedForSendingStatus': True}
        session.client.side_effect = lambda service, **kwargs: {'sts': sts, 'sesv2': ses, 'cognito-idp': cognito}[service]
        return session, ses, cognito

    def test_different_account_cannot_create_resources(self):
        session, ses, cognito = self.session('other')
        with tempfile.TemporaryDirectory() as directory, self.assertRaisesRegex(RuntimeError, 'account differs'):
            provisioner.provision(session, 'selected', 'us-east-1', 'dev', 'test@example.invalid', Path(directory)/'state.json')
        ses.get_email_identity.assert_not_called()
        cognito.create_user_pool.assert_not_called()

    def test_unverified_sender_cannot_create_pool(self):
        session, ses, cognito = self.session()
        ses.get_email_identity.return_value = {'VerifiedForSendingStatus': False}
        with tempfile.TemporaryDirectory() as directory, self.assertRaisesRegex(RuntimeError, 'Verify the SES'):
            provisioner.provision(session, 'selected', 'us-east-1', 'dev', 'test@example.invalid', Path(directory)/'state.json')
        cognito.create_user_pool.assert_not_called()

    def test_existing_pool_without_ownership_tag_is_not_modified(self):
        session, ses, cognito = self.session()
        cognito.get_paginator.return_value.paginate.return_value = [{'UserPools': [{'Name': 'dev', 'Id': 'pool'}]}]
        cognito.describe_user_pool.return_value = {'UserPool': {'Id': 'pool', 'UserPoolTags': {'Project': 'other'}}}
        with tempfile.TemporaryDirectory() as directory, self.assertRaisesRegex(RuntimeError, 'not owned'):
            provisioner.provision(session, 'selected', 'us-east-1', 'dev', 'test@example.invalid', Path(directory)/'state.json')
        cognito.set_user_pool_mfa_config.assert_not_called()
        cognito.create_user_pool.assert_not_called()


if __name__ == '__main__':
    unittest.main()
