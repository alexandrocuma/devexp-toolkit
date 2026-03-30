#!/usr/bin/env python3
"""Generate ~/.openviking/ov.conf.

Usage: python3 gen_ov_conf.py <conf_file> <vlm_key> <vlm_model> <jina_port>
"""
import json, sys, os

conf_file = sys.argv[1]
vlm_key   = sys.argv[2]
vlm_model = sys.argv[3]
jina_port = sys.argv[4]

conf = {
    "storage": {
        "workspace": os.path.expanduser("~/.openviking/data"),
        "vectordb": {"name": "context", "backend": "local", "project": "default"},
        "agfs": {"port": 1833, "log_level": "warn", "backend": "local", "timeout": 10, "retry_times": 3}
    },
    "embedding": {
        "dense": {
            "provider":  "openai",
            "model":     "jinaai/jina-embeddings-v2-base-en",
            "api_key":   "local",
            "api_base":  f"http://localhost:{jina_port}",
            "dimension": 768
        }
    },
    "vlm": {
        "provider":    "litellm",
        "model":       vlm_model,
        "api_key":     vlm_key,
        "temperature": 0.0,
        "max_retries": 2,
        "thinking":    False
    },
    "auto_generate_l0": True,
    "auto_generate_l1": True,
    "default_search_mode": "thinking",
    "default_search_limit": 3,
    "enable_memory_decay": True,
    "log": {"level": "INFO", "output": "stdout"}
}

os.makedirs(os.path.dirname(conf_file), exist_ok=True)
with open(conf_file, 'w') as f:
    json.dump(conf, f, indent=2)

print(f"  Generated: {conf_file}")
