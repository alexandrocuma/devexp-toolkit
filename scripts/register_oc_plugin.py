#!/usr/bin/env python3
"""Register the devexp opencode plugin path in config.json.

Usage: python3 register_oc_plugin.py <config_path> <plugin_path>
"""
import json, sys, os

config_path = sys.argv[1]
plugin_path = sys.argv[2]

config = {}
if os.path.exists(config_path):
    with open(config_path) as f:
        try:
            config = json.load(f)
        except json.JSONDecodeError:
            config = {}
if 'plugin' not in config:
    config['plugin'] = []

if plugin_path in config['plugin']:
    print(f"  [skip] plugin already registered in {config_path}")
else:
    config['plugin'].append(plugin_path)
    os.makedirs(os.path.dirname(config_path), exist_ok=True)
    with open(config_path, 'w') as f:
        json.dump(config, f, indent=2)
    print(f"  + registered plugin in {config_path}")
