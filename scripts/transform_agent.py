#!/usr/bin/env python3
"""Transform a Claude Code agent file to opencode format.

Usage:
  python3 transform_agent.py <src_file> [selected_model]             # full transform
  python3 transform_agent.py <src_file> [selected_model] --model-only  # model line only
"""
import re, sys

OPENCODE_TOOLS = {'read', 'write', 'edit', 'bash', 'glob', 'grep', 'webfetch', 'websearch'}

CLAUDE_TO_OC = {
    'read': 'read', 'write': 'write', 'edit': 'edit', 'bash': 'bash',
    'glob': 'glob', 'grep': 'grep', 'webfetch': 'webfetch', 'websearch': 'websearch',
}

MODEL_MAP = {
    # Anthropic
    'sonnet':       'anthropic/claude-sonnet-4-6',
    'opus':         'anthropic/claude-opus-4-6',
    'haiku':        'anthropic/claude-haiku-4-5-20251001',
    # OpenAI
    'gpt4':         'openai/gpt-4.1-2025-04-14',
    'gpt4o':        'openai/gpt-4o',
    'o3':           'openai/o3-2025-04-16',
    'o4mini':       'openai/o4-mini-2025-04-16',
    # DeepSeek
    'deepseek':     'deepseek/deepseek-chat',
    'deepseek-r1':  'deepseek/deepseek-reasoner',
    # Kimi (Moonshot)
    'kimi':         'moonshot/kimi-k2.5',
    'kimi-turbo':   'moonshot/kimi-k2-turbo-preview',
}

SKIP_KEYS = {'name', 'color', 'memory'}

src_file       = sys.argv[1]
selected_model = sys.argv[2] if len(sys.argv) > 2 and sys.argv[2] and not sys.argv[2].startswith('--') else None
model_only     = '--model-only' in sys.argv

with open(src_file) as f:
    content = f.read()

# --model-only: just substitute the model line (used for opencode-exclusive agents)
if model_only:
    if selected_model:
        resolved = MODEL_MAP.get(selected_model, selected_model)
        content = re.sub(r'^model:.*$', f'model: {resolved}', content, flags=re.MULTILINE)
    sys.stdout.write(content)
    sys.exit(0)

# Full transform: remap frontmatter fields, tools, and add mode: subagent
parts = content.split('---', 2)
if len(parts) < 3:
    sys.stdout.write(content)
    sys.exit(0)

_, fm, body = parts
new_lines = []

for line in fm.strip().split('\n'):
    m = re.match(r'^([a-zA-Z_]+)\s*:(.*)', line)
    if not m:
        new_lines.append(line)
        continue

    key = m.group(1).lower()
    val = m.group(2).strip()

    if key in SKIP_KEYS:
        continue
    elif key == 'model':
        alias = selected_model if selected_model else val
        new_lines.append(f'model: {MODEL_MAP.get(alias, alias)}')
    elif key == 'tools':
        claude_tools = {t.strip().lower() for t in val.split(',')}
        enabled  = {CLAUDE_TO_OC[t] for t in claude_tools if t in CLAUDE_TO_OC}
        disabled = sorted(OPENCODE_TOOLS - enabled)
        if disabled:
            new_lines.append('tools:')
            for t in disabled:
                new_lines.append(f'  {t}: false')
        # all opencode tools enabled → omit section (default = all on)
    else:
        new_lines.append(line)

new_lines.append('mode: subagent')
sys.stdout.write('---\n' + '\n'.join(new_lines) + '\n---' + body)
