#!/usr/bin/env python3
"""Register extra MCP servers from devexp.config.json in Claude Code.

Usage: python3 install_extra_mcps_claude.py <extra_mcps_json> <dotenv_path> <dry_run:0|1>
"""
import json, sys, subprocess, os

mcps        = json.loads(sys.argv[1])
dotenv_path = sys.argv[2]
dry_run     = sys.argv[3] == "1"

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

for mcp in mcps:
    name         = mcp['name']
    command      = mcp['command']
    args         = mcp.get('args', [])
    scope        = mcp.get('scope', 'user')
    env_vars     = mcp.get('env', {})
    required_env = mcp.get('required_env', [])

    resolved = {k: merged_env.get(k, v) for k, v in env_vars.items()}
    for key in required_env:
        if key in merged_env and key not in resolved:
            resolved[key] = merged_env[key]

    missing = [e for e in required_env if not merged_env.get(e)]
    if missing:
        print(f"  [skip] {name} — missing env: {', '.join(missing)}")
        continue

    env_flags = [item for k, v in resolved.items() for item in ('--env', f'{k}={v}')]

    if dry_run:
        e = ' '.join(f'--env {k}=***' for k in resolved) if resolved else ''
        print(f"  [dry-run] claude mcp add --scope {scope} {e} {name} -- {command} {' '.join(args)}")
        continue

    result = subprocess.run(['claude', 'mcp', 'list'], capture_output=True, text=True)
    if name in result.stdout:
        print(f"  [skip] {name} — already installed")
        continue

    r = subprocess.run(
        ['claude', 'mcp', 'add', '--scope', scope] + env_flags + [name, '--', command] + args,
        capture_output=True, text=True
    )
    if r.returncode == 0:
        print(f"  \033[0;32m+\033[0m {name}")
    else:
        print(f"  \033[1;33m[warn]\033[0m {name} — {r.stderr.strip()}", file=sys.stderr)
