#!/usr/bin/env python3
"""Build/check every Go module with GOWORK=off; app manifests are language-neutral."""
import argparse
import json
import os
from pathlib import Path
import re
import subprocess

ROOT = Path(__file__).resolve().parents[1]

def modules():
    return [ROOT, *sorted((ROOT / 'packages/go').glob('*/go.mod')), *sorted((ROOT / 'apps').glob('*/go.mod'))]

def boundaries():
    for area in ['apps', 'packages/go']:
        for source in (ROOT / area).rglob('*.go'):
            if source.name.endswith('_test.go'):
                continue
            for imp in re.findall(r'"(stsmodel\.local/[^"\n]+)"', source.read_text()):
                if imp.startswith('stsmodel.local/testkit'):
                    raise RuntimeError(f'Production imports testkit: {source}')
                domain_dirs = ['internal/actions', 'internal/conversation', 'internal/notes', 'internal/scheduling']
                if any(part in source.as_posix() for part in domain_dirs) and '/internal/adapters/' in imp:
                    raise RuntimeError(f'Domain imports adapter: {source}: {imp}')
                if '/internal/' in imp:
                    owner = imp.split('/')[1]
                    if not source.is_relative_to(ROOT / 'apps' / owner):
                        raise RuntimeError(f'Cross-application import: {source}: {imp}')
                if area == 'packages/go' and any(imp.startswith('stsmodel.local/' + app + '/') for app in ['voice-gateway', 'mcp-notes', 'mcp-agenda', 'voice-client']):
                    raise RuntimeError(f'Technical library imports app: {source}')
    print('Application boundaries OK', flush=True)

def build_apps(env):
    (ROOT / 'bin').mkdir(exist_ok=True)
    for path in sorted((ROOT / 'apps').glob('*/app.json')):
        app = json.loads(path.read_text())
        subprocess.run(app['build'], cwd=path.parent, env=env, check=True)

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=['build', 'test', 'race', 'vet', 'boundaries'])
    args = parser.parse_args()
    boundaries()
    if args.action == 'boundaries':
        return
    env = dict(os.environ, GOWORK='off', STS_TEST_BIN_DIR=str(ROOT / 'bin'))
    env.setdefault('GOCACHE', '/private/tmp/sts-go-cache')
    if args.action in ['build', 'test', 'race']:
        build_apps(env)
    for entry in modules():
        directory = entry.parent if entry.name == 'go.mod' else entry
        print(f'{args.action}: {directory.relative_to(ROOT)}', flush=True)
        if args.action == 'build' and directory.is_relative_to(ROOT / 'apps'):
            relative = directory.relative_to(ROOT).as_posix().replace('/', '-') or 'root'
            output = Path('/private/tmp/sts-module-build') / relative
            output.mkdir(parents=True, exist_ok=True)
            command = ['build', '-o', str(output) + os.sep]
        elif args.action == 'race':
            command = ['test', '-race']
        else:
            command = [args.action]
        subprocess.run(['go', *command, './...'], cwd=directory, env=env, check=True)

if __name__ == '__main__':
    main()
