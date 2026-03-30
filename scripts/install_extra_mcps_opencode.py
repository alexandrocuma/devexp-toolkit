#!/usr/bin/env python3
"""Register extra MCP servers from devexp.config.json in opencode's config.json.

Usage: python3 install_extra_mcps_opencode.py <extra_mcps_json> <config_path> <dotenv_path> <dry_run:0|1>
"""
import json, sys, os

mcps        = json.loads(sys.argv[1])
config_path = sys.argv[2]
dotenv_path = sys.argv[3]
dry_run     = sys.argv[4] == "1"

if not mcps:
    sys.exit(0)

dotenv = {}
if os.path.exists(dotenv_path):
    with open(dotenv_path) as f:
        for line in f:
            line = line.strip()
            if line and not line.startswith('#') and '=' in line:
                k, _, v = line.partition('=')
                dotenv[k.strip()] = v.strip()

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
    name         = mcp['name']
    command      = mcp['command']
    args         = mcp.get('args', [])
    env_vars     = mcp.get('env', {})
    required_env = mcp.get('required_env', [])

    resolved = {k: merged_env.get(k, v) for k, v in env_vars.items()}
    for key in required_env:
        if key in merged_env and key not in resolved:
            resolved[key] = merged_env[key]

    missing = [e for e in required_env if not merged_env.get(e)]
    if missing:
        print(f"\n  \033[0;31m[REQUIRED]\033[0m {name} — missing required env vars:")
        for key in missing:
            print(f"    {key}=<your-value>")
        print(f"\n  {name} will not be available until these are set.\n")
        continue

    entry = {'type': 'local', 'command': [command] + args}
    if resolved:
        entry['env'] = resolved

    if dry_run:
        print(f"  [dry-run] add mcp.{name} to {config_path}")
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
