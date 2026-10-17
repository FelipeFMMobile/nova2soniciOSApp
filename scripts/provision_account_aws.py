#!/usr/bin/env python3
"""Provision the authorized, tagged Cognito development pool. No credentials in output."""
import argparse
import json
import os
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


def provision(session, account, region, prefix, sender, state_path):
    actual = session.client('sts').get_caller_identity()['Account']
    if actual != account:
        raise RuntimeError('AWS account differs from the explicitly selected account.')
    ses = session.client('sesv2', region_name=region)
    if not ses.get_email_identity(EmailIdentity=sender)['VerifiedForSendingStatus']:
        raise RuntimeError('Verify the SES sender before provisioning Cognito.')
    cognito = session.client('cognito-idp', region_name=region)
    state = {'account': account, 'region': region, 'prefix': prefix, 'sender': sender}

    def save():
        state_path.parent.mkdir(parents=True, exist_ok=True)
        temporary = state_path.with_suffix('.tmp')
        fd = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
        os.fchmod(fd, 0o600)
        with os.fdopen(fd, 'w') as handle:
            json.dump(state, handle, indent=2)
        temporary.replace(state_path)

    matches = [pool for page in cognito.get_paginator('list_user_pools').paginate(MaxResults=60)
               for pool in page['UserPools'] if pool['Name'] == prefix]
    if len(matches) > 1:
        raise RuntimeError('Ambiguous pool name; no existing pools were modified.')
    if matches:
        pool = cognito.describe_user_pool(UserPoolId=matches[0]['Id'])['UserPool']
        if pool.get('UserPoolTags', {}).get('Project') != prefix:
            raise RuntimeError('Existing pool is not owned by this development project.')
    else:
        pool = cognito.create_user_pool(
            PoolName=prefix, UserPoolTier='ESSENTIALS', DeletionProtection='ACTIVE',
            UserPoolTags={'Project': prefix, 'Environment': 'development'},
            Policies={'PasswordPolicy': {'MinimumLength': 12, 'RequireUppercase': True,
                      'RequireLowercase': True, 'RequireNumbers': True, 'RequireSymbols': True,
                      'TemporaryPasswordValidityDays': 7}},
            UsernameConfiguration={'CaseSensitive': False}, AliasAttributes=['email'],
            AutoVerifiedAttributes=['email'],
            Schema=[{'Name': 'email', 'AttributeDataType': 'String', 'Required': True, 'Mutable': True}],
            AdminCreateUserConfig={'AllowAdminCreateUserOnly': True},
            AccountRecoverySetting={'RecoveryMechanisms': [{'Priority': 1, 'Name': 'admin_only'}]},
            EmailConfiguration={'EmailSendingAccount': 'DEVELOPER',
                                'SourceArn': f'arn:aws:ses:{region}:{account}:identity/{sender}',
                                'From': sender},
            MfaConfiguration='OFF')['UserPool']
    pool_id = pool['Id']
    state['user_pool_id'] = pool_id
    save()
    # No app client/users exist while MFA is initially configured.
    cognito.set_user_pool_mfa_config(UserPoolId=pool_id, MfaConfiguration='ON',
        EmailMfaConfiguration={'Subject': 'Seu código de acesso — STS Conta',
                               'Message': 'Seu código de autenticação é {####}.'})
    state['email_mfa_required'] = True
    save()
    name = prefix + '-apple'
    clients = [client for page in cognito.get_paginator('list_user_pool_clients').paginate(UserPoolId=pool_id, MaxResults=60)
               for client in page['UserPoolClients'] if client['ClientName'] == name]
    if len(clients) > 1:
        raise RuntimeError('Ambiguous client name; no clients were modified.')
    if clients:
        client = cognito.describe_user_pool_client(UserPoolId=pool_id, ClientId=clients[0]['ClientId'])['UserPoolClient']
    else:
        client = cognito.create_user_pool_client(UserPoolId=pool_id, ClientName=name,
            GenerateSecret=False, SupportedIdentityProviders=['COGNITO'],
            AllowedOAuthFlowsUserPoolClient=True, AllowedOAuthFlows=['code'],
            AllowedOAuthScopes=['openid', 'email', 'aws.cognito.signin.user.admin'],
            CallbackURLs=['stsvoice://oauth/callback'],
            ExplicitAuthFlows=['ALLOW_USER_SRP_AUTH', 'ALLOW_REFRESH_TOKEN_AUTH'],
            PreventUserExistenceErrors='ENABLED', EnableTokenRevocation=True,
            AccessTokenValidity=15, IdTokenValidity=15, RefreshTokenValidity=30,
            TokenValidityUnits={'AccessToken': 'minutes', 'IdToken': 'minutes', 'RefreshToken': 'days'},
            ReadAttributes=['email', 'email_verified', 'sub'], WriteAttributes=['email'])['UserPoolClient']
    if client.get('ClientSecret') or client.get('CallbackURLs') != ['stsvoice://oauth/callback']:
        raise RuntimeError('Existing app client configuration differs; no client was overwritten.')
    state['client_id'] = client['ClientId']
    save()
    domain = prefix + '-' + account[-6:]
    pool = cognito.describe_user_pool(UserPoolId=pool_id)['UserPool']
    if pool.get('Domain') not in (None, '', domain):
        raise RuntimeError('Existing pool has a different domain; no domain was replaced.')
    if not pool.get('Domain'):
        cognito.create_user_pool_domain(UserPoolId=pool_id, Domain=domain, ManagedLoginVersion=2)
    state['domain'] = f'https://{domain}.auth.{region}.amazoncognito.com'
    state['issuer'] = f'https://cognito-idp.{region}.amazonaws.com/{pool_id}'
    save()
    try:
        cognito.describe_managed_login_branding_by_client(UserPoolId=pool_id, ClientId=client['ClientId'])
    except cognito.exceptions.ResourceNotFoundException:
        cognito.create_managed_login_branding(UserPoolId=pool_id, ClientId=client['ClientId'], UseCognitoProvidedValues=True)
    state['managed_login_configured'] = True
    save()
    print(json.dumps(state, indent=2))
    return state


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--profile', default='default')
    parser.add_argument('--account', required=True)
    parser.add_argument('--sender', required=True)
    parser.add_argument('--region', default='us-east-1')
    parser.add_argument('--prefix', default='sts-account-dev')
    parser.add_argument('--state', type=Path, default=ROOT / 'data/account/aws-dev-resources.json')
    args = parser.parse_args()
    import boto3
    from botocore.exceptions import ClientError
    try:
        provision(boto3.Session(profile_name=args.profile, region_name=args.region),
                  args.account, args.region, args.prefix, args.sender, args.state)
    except ClientError as error:
        # AWS response bodies can include sensitive input; retain only operation/code.
        print(f"AWS {error.operation_name}: {error.response['Error']['Code']}.")
        if args.state.exists():
            print('Resource progress is saved in the private state file.')
        if error.response['Error']['Code'] == 'AccessDeniedException':
            print(error.response['Error'].get('Message', 'Permission denied.'))
        return 1
    except RuntimeError as error:
        print(str(error))
        return 1
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
