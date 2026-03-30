#!/usr/bin/env python3
"""Register MCP servers in opencode's config.json.

Usage: python3 install_mcps_opencode.py <registry> <config_path> <dotenv_path> <dry_run:0|1> [skipped_mcps_file]
"""
import json, sys, os, re

with open(sys.argv[1]) as f:
    mcps = json.load(f)
config_path       = sys.argv[2]
dotenv_path       = sys.argv[3]
dry_run           = sys.argv[4] == "1"
skipped_mcps_file = sys.argv[5] if len(sys.argv) > 5 else ""

dotenv = {}
if os.path.exists(dotenv_path):
    with open(dotenv_path) as f:
        for line in f:
            line = line.strip()
            if line and not line.startswith('#') and '=' in line:
                k, _, v = line.partition('=')
                dotenv[k.strip()] = v.strip()
    if dotenv:
        print(f"  Loaded {len(dotenv)} var(s) from mcps/.env")

merged_env = {**os.environ, **dotenv}

config = {}
if os.path.exists(config_path):
    with open(config_path) as f:
        try:
            config = json.load(f)
        except json.JSONDecodeError:
            config = {}
if 'mcp' not in config:
    config['mcp'] = {}

added = []
for mcp in mcps:
    name               = mcp['name']
    transport          = mcp.get('transport', 'stdio')
    url                = mcp.get('url', '')
    command            = mcp.get('command', '')
    args               = mcp.get('args', [])
    env_vars           = mcp.get('env', {})
    required_env       = mcp.get('required_env', [])
    setup_instructions = mcp.get('setup_instructions', '')
    headers            = mcp.get('headers', {})

    resolved = {k: merged_env.get(k, v) for k, v in env_vars.items()}
    for key in required_env:
        if key in merged_env and key not in resolved:
            resolved[key] = merged_env[key]

    def resolve_str(s):
        return re.sub(r'\$\{(\w+)\}', lambda m: merged_env.get(m.group(1), ''), s)
    resolved_headers = {k: resolve_str(v) for k, v in headers.items()}

    missing = [e for e in required_env if not merged_env.get(e)]
    if missing:
        print(f"\n  \033[0;31m[REQUIRED]\033[0m {name} — missing required env vars:")
        for key in missing:
            print(f"    {key}=<your-value>")
        if setup_instructions:
            print()
            for line in setup_instructions.split('\n'):
                print(f"  {line}")
        print(f"\n  {name} will not be available until these are set.\n")
        if skipped_mcps_file:
            with open(skipped_mcps_file, 'a') as f:
                f.write(name + '\n')
        continue

    if transport in ('sse', 'http'):
        entry = {'type': 'remote', 'url': url}
        if resolved_headers:
            entry['headers'] = resolved_headers
    else:
        entry = {'type': 'local', 'command': [command] + args}
        if resolved:
            entry['env'] = resolved

    if dry_run:
        print(f"  [dry-run] add mcp.{name} ({transport}) to {config_path}")
        continue

    if name in config['mcp']:
        if config['mcp'][name] == entry:
            print(f"  [skip] {name} — already configured")
            continue
        config['mcp'][name] = entry
        added.append(name)
        print(f"  \033[0;33m~\033[0m {name} — updated")
        continue

    config['mcp'][name] = entry
    added.append(name)
    print(f"  \033[0;32m+\033[0m {name}")

if added and not dry_run:
    os.makedirs(os.path.dirname(config_path), exist_ok=True)
    with open(config_path, 'w') as f:
        json.dump(config, f, indent=2)
    print(f"  Saved: {config_path}")
