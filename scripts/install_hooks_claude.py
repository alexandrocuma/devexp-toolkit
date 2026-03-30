#!/usr/bin/env python3
"""Register devexp hooks in Claude Code's settings.json.

Usage: python3 install_hooks_claude.py <registry> <repo_dir> <settings_path> <dry_run:0|1> [disabled_csv]
"""
import json, sys, os

registry_path = sys.argv[1]
repo_dir      = sys.argv[2]
settings_path = sys.argv[3]
dry_run       = sys.argv[4] == "1"
disabled      = set(filter(None, sys.argv[5].split(','))) if len(sys.argv) > 5 else set()

with open(registry_path) as f:
    hooks = json.load(f)

settings = {}
if os.path.exists(settings_path):
    with open(settings_path) as f:
        try:
            settings = json.load(f)
        except json.JSONDecodeError:
            settings = {}
if 'hooks' not in settings:
    settings['hooks'] = {}

changed = False
for hook in hooks:
    if not hook.get('enabled', True):
        continue
    if hook.get('name') in disabled:
        print(f"  [skip] {hook['name']} (disabled in devexp.config.json)")
        continue
    cc     = hook.get('claude_code', {})
    event  = cc.get('event')
    script = cc.get('script')
    if not event or not script:
        continue

    script_abs = os.path.join(repo_dir, script)

    if dry_run:
        print(f"  [dry-run] add {event} hook: {os.path.basename(script)}")
        continue

    if event not in settings['hooks']:
        settings['hooks'][event] = []

    existing_cmds = [
        h.get('hooks', [{}])[0].get('command', '')
        for h in settings['hooks'][event]
        if h.get('hooks')
    ]
    if script_abs in existing_cmds:
        print(f"  [skip] {event}: {os.path.basename(script)} — already registered")
        continue

    settings['hooks'][event].append({
        'matcher': cc.get('matcher', '.*'),
        'hooks': [{'type': 'command', 'command': script_abs}]
    })
    changed = True
    print(f"  \033[0;32m+\033[0m {event}: {os.path.basename(script)}")

if changed and not dry_run:
    os.makedirs(os.path.dirname(settings_path), exist_ok=True)
    with open(settings_path, 'w') as f:
        json.dump(settings, f, indent=2)
    print(f"  Saved: {settings_path}")
