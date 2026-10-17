#!/usr/bin/env python3
"""Run fake Account integration in an isolated stack; optionally exercise iOS UI."""
from __future__ import annotations
import argparse
import json
import os
from pathlib import Path
import plistlib
import secrets
import socket
import subprocess
import tempfile

import dev
import dev_account as account

ROOT = Path(__file__).resolve().parents[1]


def free_ports(count):
    sockets = []
    try:
        for _ in range(count):
            listener = socket.socket()
            listener.bind(('127.0.0.1', 0))
            sockets.append(listener)
        return [listener.getsockname()[1] for listener in sockets]
    finally:
        for listener in sockets:
            listener.close()


def clean_environment():
    return {key: value for key, value in os.environ.items()
            if not key.startswith(('STS_', 'AWS_', 'NOVA_'))}


def run(command, env, log):
    with log.open('ab') as output:
        result = subprocess.run(command, cwd=ROOT, env=env, stdout=output, stderr=subprocess.STDOUT)
    if result.returncode:
        raise RuntimeError(f'Command failed ({result.returncode}); inspect {log}.')


def xctest_environment(document, values):
    """Version 1/2 xctestrun layouts; only test-runner environment is changed."""
    targets = []
    if 'TestConfigurations' in document:
        for configuration in document['TestConfigurations']:
            targets.extend(configuration.get('TestTargets', []))
    else:
        targets = [value for key, value in document.items()
                   if key != '__xctestrun_metadata__' and isinstance(value, dict)]
    selected = [target for target in targets if target.get('BlueprintName') == 'NovaVoiceUITests'
                or 'NovaVoiceUITests' in target.get('TestBundlePath', '')]
    if not selected:
        raise RuntimeError('NovaVoiceUITests missing from xctestrun.')
    for target in selected:
        target.setdefault('EnvironmentVariables', {}).update(values)
    return document


def assert_private(content, forbidden):
    if any(value and value.encode() in content for value in forbidden):
        # Never repeat the matched value or the source content in an error.
        raise RuntimeError('Privacy audit failed: sensitive canary found in diagnostics.')


def audit_privacy(compose, env, work, forbidden):
    sources = [compose + ['logs', '--no-color'],
               compose + ['exec', '-T', 'usage-db', 'psql', '-U', 'litellm', '-d', 'litellm',
                          '-At', '-c', 'SELECT row_to_json(s)::text FROM "LiteLLM_SpendLogs" s;']]
    spend_rows = 0
    for index, command in enumerate(sources):
        result = subprocess.run(command, cwd=ROOT, env=env, capture_output=True)
        if result.returncode:
            raise RuntimeError('Privacy audit unavailable; diagnostics were not printed.')
        assert_private(result.stdout, forbidden)
        assert_private(result.stderr, forbidden)
        if index == 1:
            spend_rows = len(result.stdout.splitlines())
    for path in work.glob('*.log'):
        assert_private(path.read_bytes(), forbidden)
    print(f'Privacy audit passed: service logs, local diagnostics; spend rows inspected: {spend_rows}.', flush=True)
    if not spend_rows:
        print('No spend rows produced by MCP-only tests; Realtime spend audit remains pending.', flush=True)


def apple_ui(args, env, gateway_port, work, log):
    derived = Path(args.apple_build_root).resolve()
    # Preserve simulator ad hoc signing: unsigned builds cannot exercise Keychain.
    command = ['xcodebuild', '-project', str(ROOT / 'apps/apple/NovaVoice.xcodeproj'),
               '-scheme', 'NovaVoice', '-destination', 'platform=iOS Simulator,id=' + args.simulator,
               '-derivedDataPath', str(derived), '-quiet']
    print('Building iOS UI tests…', flush=True)
    run(command + ['build-for-testing'], env, log)
    candidates = sorted((derived / 'Build/Products').glob('*.xctestrun'))
    if len(candidates) != 1:
        raise RuntimeError('Expected exactly one xctestrun in the dedicated build directory.')
    source = candidates[0]
    document = plistlib.loads(source.read_bytes())
    xctest_environment(document, {'STS_ACCOUNT_UI': '1',
        'STS_TEST_URL': f'ws://127.0.0.1:{gateway_port}/v1/voice',
        'STS_TEST_TOKEN': 'apple-test-token'})
    # Keep __TESTROOT__ relative to the original build products directory.
    path = source.with_name('AccountSmoke.xctestrun')
    try:
        path.write_bytes(plistlib.dumps(document))
        print('Running iOS login, restart, logout and agent switching…', flush=True)
        run(['xcodebuild', 'test-without-building', '-xctestrun', str(path),
             '-destination', 'platform=iOS Simulator,id=' + args.simulator,
             '-parallel-testing-enabled', 'NO', '-resultBundlePath', str(work / 'apple.xcresult'), '-quiet'], env, log)
    finally:
        path.unlink(missing_ok=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--apple-ui', action='store_true')
    parser.add_argument('--simulator', default='599499E4-DD38-4C6C-9459-D4FDA8B2AE43')
    parser.add_argument('--apple-build-root', default='/private/tmp/sts-account-ui')
    parser.add_argument('--output', default='/private/tmp/sts-account-smoke')
    args = parser.parse_args()
    output = Path(args.output).resolve()
    output.mkdir(parents=True, exist_ok=True)
    work = Path(tempfile.mkdtemp(prefix='run-', dir=output))
    work.chmod(0o700)
    env = clean_environment()
    env.setdefault('GOCACHE', '/private/tmp/sts-go-cache')
    env.setdefault('GOWORK', str(ROOT / 'go.work'))
    identity_port, profile_port, proxy_port, gateway_port, personal_port, agent_port = free_ports(6)
    settings = {key: ('sk-' if key == 'LITELLM_MASTER_KEY' else '') + secrets.token_hex(32)
                for key in account.SECRET_KEYS}
    settings.update(STS_IDENTITY_PROVIDER='fake', STS_IDENTITY_PORT=str(identity_port),
                    STS_PROFILE_PORT=str(profile_port), STS_ACCOUNT_LITELLM_PORT=str(proxy_port))
    env_file = work / '.env'
    account.save_settings(settings, env_file)
    project = 'sts-account-smoke-' + secrets.token_hex(4)
    privacy_canary = 'privacy-canary-' + secrets.token_hex(16)
    compose = ['docker', 'compose', '-p', project, '--env-file', str(env_file),
               '-f', str(ROOT / 'deploy/account/compose.yaml')]
    log = work / 'smoke.log'
    children, handles = [], []
    print(f'Isolated fake stack: {project}; evidence: {work}', flush=True)
    try:
        run(compose + ['up', '--build', '-d'], env, log)
        identity_url, profile_url, proxy_url = [f'http://127.0.0.1:{p}' for p in (identity_port, profile_port, proxy_port)]
        for url in [identity_url + '/healthz', profile_url + '/healthz', proxy_url + '/health/liveliness']:
            account.wait_http(url)
        account.seed(settings, 'unused', [], identity_url, profile_url)
        key = account.service_key(proxy_url, settings['LITELLM_MASTER_KEY'], 'sts-account', {
            'sts-identity': ['identity.begin_login', 'identity.login_status', 'identity.logout'],
            'sts-profile': ['profile.get_me', 'profile.update_me']})
        integration = dict(env, STS_ACCOUNT_INTEGRATION='1', STS_LITELLM_URL=proxy_url,
            STS_LITELLM_API_KEY=key, STS_MCP_CONTEXT_SECRET=settings['STS_ACCOUNT_CONTEXT_SECRET'],
            STS_TEST_IDENTITY_URL=identity_url, STS_TEST_IDENTITY_TOKEN=settings['STS_IDENTITY_SERVICE_TOKEN'],
            STS_TEST_PROFILE_URL=profile_url, STS_TEST_PROFILE_TOKEN=settings['STS_PROFILE_SERVICE_TOKEN'],
            STS_TEST_PRIVACY_CANARY=privacy_canary)
        print('Running real Go → LiteLLM → Spring and AgentSession v2 tests…', flush=True)
        run(['go', 'test', './apps/account-agent/internal/account', '-run', 'TestLive', '-count=1', '-v'], integration, log)
        personal_key = account.service_key(proxy_url, settings['LITELLM_MASTER_KEY'], 'sts-personal', {
            'sts-notes': ['notes.create', 'notes.list', 'notes.delete'],
            'sts-agenda': ['agenda.list_slots', 'agenda.list_events', 'agenda.create_event', 'agenda.cancel_event']})
        legacy = dict(env, STS_LITELLM_E2E='1', STS_LITELLM_URL=proxy_url,
                      STS_LITELLM_API_KEY=personal_key, STS_MCP_CONTEXT_SECRET=settings['STS_LEGACY_CONTEXT_SECRET'],
                      STS_TEST_PRIVACY_CANARY=privacy_canary)
        print('Running Agenda/notes MCP v1 regression in the shared composition…', flush=True)
        run(['go', 'test', './apps/personal-assistant-agent/internal/adapters/mcpclient',
             '-run', '^TestLiteLLMLocalGateway$', '-count=1', '-v'], legacy, log)
        if args.apple_ui:
            app_env = dev.environment('fake', 'unused', False, 'apple-test-token', [], gateway_port)
            app_env['STS_AGENT_ADDRESS'] = f'127.0.0.1:{personal_port}'
            edge, personal = dev.runtime_environments(app_env)
            agent_token = secrets.token_hex(32)
            agent = dict(env, STS_PROVIDER='fake', STS_AGENT_ADDRESS=f'127.0.0.1:{agent_port}',
                STS_AGENT_TOKEN=agent_token, STS_AGENT_PROMPT_FILE=str(ROOT / 'apps/account-agent/prompts/system.md'),
                STS_LITELLM_URL=proxy_url, STS_LITELLM_API_KEY=key,
                STS_MCP_CONTEXT_SECRET=settings['STS_ACCOUNT_CONTEXT_SECRET'])
            catalog = json.loads(edge['STS_AGENT_CATALOG'])
            catalog.append({'id': 'account', 'endpoint': f'ws://127.0.0.1:{agent_port}/v2/session',
                            'protocol': 'agent-session/v2', 'token_env': 'STS_AGENT_TOKEN_ACCOUNT'})
            edge.update(STS_AGENT_CATALOG=json.dumps(catalog), STS_AGENT_TOKEN_ACCOUNT=agent_token,
                        STS_IDENTITY_URL=identity_url, STS_IDENTITY_SERVICE_TOKEN=settings['STS_IDENTITY_SERVICE_TOKEN'])
            for name, target, child_env, port in [
                ('personal', './apps/personal-assistant-agent/cmd/agent', personal, personal_port),
                ('account', './apps/account-agent/cmd/account-agent', agent, agent_port),
                ('gateway', './apps/voice-gateway/cmd/gateway', edge, gateway_port)]:
                binary = work / name
                run(['go', 'build', '-o', str(binary), target], env, log)
                handle = (work / (name + '.log')).open('wb'); handles.append(handle)
                children.append(subprocess.Popen([str(binary)], cwd=ROOT, env=child_env,
                    stdin=subprocess.DEVNULL, stdout=handle, stderr=handle, start_new_session=True))
                dev.wait_ready(children, port)
            apple_ui(args, env, gateway_port, work, log)
        audit_privacy(compose, env, work, [privacy_canary, key, personal_key] +
                      [settings[name] for name in account.SECRET_KEYS])
        print('Account smoke passed.', flush=True)
        return 0
    finally:
        dev.stop_children(children)
        for handle in handles:
            handle.close()
        # Only this invocation's randomly named test resources are removed.
        cleanup = subprocess.run(compose + ['down', '--volumes', '--remove-orphans'],
                                 cwd=ROOT, env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        if cleanup.returncode:
            print(f'Cleanup failed for isolated stack {project}; retry docker compose down locally.')
        env_file.unlink(missing_ok=True)


if __name__ == '__main__':
    try:
        raise SystemExit(main())
    except (RuntimeError, KeyboardInterrupt) as error:
        print(str(error) or 'Interrupted.')
        raise SystemExit(1)
