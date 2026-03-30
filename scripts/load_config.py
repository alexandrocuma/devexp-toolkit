#!/usr/bin/env python3
"""Load devexp.config.json and print bash variable assignments to stdout.

Usage: python3 load_config.py <config_file>
"""
import json, sys

try:
    with open(sys.argv[1]) as f:
        cfg = json.load(f)
except Exception as e:
    print(f'warn:devexp.config.json parse error: {e}', file=sys.stderr)
    cfg = {}

def bash_arr(items):
    return '(' + ' '.join(f'"{i}"' for i in (items or [])) + ')'

print(f'CONFIG_DISABLED_AGENTS={bash_arr(cfg.get("agents", {}).get("disabled", []))}')
print(f'CONFIG_DISABLED_SKILLS={bash_arr(cfg.get("skills", {}).get("disabled", []))}')
print(f'CONFIG_DISABLED_HOOKS={bash_arr(cfg.get("hooks",  {}).get("disabled", []))}')
print(f'CONFIG_MODEL="{cfg.get("model") or ""}"')
print(f'CONFIG_EXTRA_MCPS={repr(json.dumps(cfg.get("mcps", [])))}')
