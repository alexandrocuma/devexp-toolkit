#!/usr/bin/env python3
"""Register MCP servers in Claude Code via `claude mcp add`.

Usage: python3 install_mcps_claude.py <registry> <dotenv_path> <dry_run:0|1> [skipped_mcps_file]
"""
import json, sys, subprocess, os, re

with open(sys.argv[1]) as f:
    mcps = json.load(f)
dotenv_path       = sys.argv[2]
dry_run           = sys.argv[3] == "1"
skipped_mcps_file = sys.argv[4] if len(sys.argv) > 4 else ""

# Load mcps/.env
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

for mcp in mcps:
    name               = mcp['name']
    transport          = mcp.get('transport', 'stdio')
    url                = mcp.get('url', '')
    command            = mcp.get('command', '')
    args               = mcp.get('args', [])
    scope              = mcp.get('scope', 'user')
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

    env_flags    = [item for k, v in resolved.items()       for item in ('--env', f'{k}={v}')]
    header_flags = [item for k, v in resolved_headers.items() for item in ('-H', f'{k}: {v}')]

    if dry_run:
        if transport in ('sse', 'http'):
            h = ' '.join(f'-H "{k}: ***"' for k in resolved_headers) if resolved_headers else ''
            print(f"  [dry-run] claude mcp add --scope {scope} --transport {transport} {h} {name} {url}")
        else:
            e = ' '.join(f'--env {k}=***' for k in resolved) if resolved else ''
            print(f"  [dry-run] claude mcp add --scope {scope} {e} {name} -- {command} {' '.join(args)}")
        continue

    result = subprocess.run(['claude', 'mcp', 'list'], capture_output=True, text=True)
    if name in result.stdout:
        print(f"  [skip] {name} — already installed")
        continue

    if transport in ('sse', 'http'):
        r = subprocess.run(
            ['claude', 'mcp', 'add', '--scope', scope, '--transport', transport] + header_flags + [name, url],
            capture_output=True, text=True
        )
    else:
        r = subprocess.run(
            ['claude', 'mcp', 'add', '--scope', scope] + env_flags + [name, '--', command] + args,
            capture_output=True, text=True
        )
    if r.returncode == 0:
        print(f"  \033[0;32m+\033[0m {name}")
    else:
        print(f"  \033[1;33m[warn]\033[0m {name} — {r.stderr.strip()}", file=sys.stderr)
