#!/usr/bin/env python3
"""Print docker-compose services to start (name|compose_rel), one per line.
Skips entries missing docker_compose field or with unmet required_env.

Usage: python3 docker_services.py <registry> <dotenv_path>
"""
import json, sys, os

with open(sys.argv[1]) as f:
    mcps = json.load(f)
dotenv_path = sys.argv[2]

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
    dc = mcp.get('docker_compose')
    if not dc:
        continue
    required_env = mcp.get('required_env', [])
    if any(not merged_env.get(e) for e in required_env):
        continue
    print(f"{mcp['name']}|{dc}")
