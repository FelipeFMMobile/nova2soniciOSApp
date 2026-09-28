#!/usr/bin/env python3
"""Validate vendored skill and emit agent context; no LLM invocation."""
import argparse
import hashlib
import json
from pathlib import Path


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--check', action='store_true', help='validate only')
    args = parser.parse_args()
    root = Path(__file__).resolve().parent
    skill = root / 'skills/domain-analysis/SKILL.md'
    source = json.loads((skill.parent / 'source.json').read_text())
    if hashlib.sha256(skill.read_bytes()).hexdigest() != source['sha256']:
        parser.exit(1, 'Skill checksum mismatch; review source.json and upstream.\n')
    files = [root / 'domain-analysis.md', root / 'agents/domain-analyst.md',
             root / 'agents/modular-architect.md', skill]
    for path in files:
        if not path.is_file():
            parser.exit(1, f'Missing harness input: {path}\n')
    if args.check:
        print(f"Harness OK: domain-analysis @ {source['revision']}")
        return
    for path in files:
        print(f'\n<!-- {path.relative_to(root)} -->\n')
        print(path.read_text())


if __name__ == '__main__':
    main()
