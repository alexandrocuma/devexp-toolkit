#!/usr/bin/env bash
# devexp hook: format-on-save
# Event: PostToolUse | Matcher: Write|Edit
# Runs the project formatter on edited source files. Modifies the file in-place.
#
# Formatter priority per language:
#   JS/TS:  local biome --write (if biome.json) > local prettier > global biome > global prettier
#   Python: ruff format > black
#   Go:     gofmt -w
#   Ruby:   rubocop --autocorrect-all
#
# Silent if no formatter is installed. Times out after 15s.

set -euo pipefail

input=$(cat)

file_path=$(echo "$input" | python3 -c \
    "import sys,json; d=json.load(sys.stdin); print(d.get('tool_input',{}).get('file_path',''))" \
    2>/dev/null || echo "")

python3 - "$file_path" <<'PYFORMAT'
import sys, os, shutil, subprocess

file_path = sys.argv[1]
if not file_path or not os.path.exists(file_path):
    sys.exit(0)

ext = os.path.splitext(file_path)[1].lower()

FORMAT_EXTS = {'.js', '.jsx', '.ts', '.tsx', '.mjs', '.cjs', '.py', '.go', '.rb'}
if ext not in FORMAT_EXTS:
    sys.exit(0)

def find_root(path):
    markers = {'package.json', 'pyproject.toml', 'go.mod', 'Cargo.toml', '.git'}
    d = os.path.dirname(os.path.abspath(path))
    while True:
        if any(os.path.exists(os.path.join(d, m)) for m in markers):
            return d
        parent = os.path.dirname(d)
        if parent == d:
            return d
        d = parent

root = find_root(file_path)

def cmd_exists(cmd):
    return shutil.which(cmd) is not None

def run_formatter(cmd, cwd=None):
    try:
        r = subprocess.run(cmd, capture_output=True, text=True, timeout=15, cwd=cwd or root)
        output = (r.stdout + r.stderr).strip()
        if output:
            print(f'[devexp format-on-save] {os.path.basename(cmd[0])}:', file=sys.stderr)
            print(output, file=sys.stderr)
    except subprocess.TimeoutExpired:
        print(f'[devexp format-on-save] {os.path.basename(cmd[0])} timed out (>15s)', file=sys.stderr)
    except FileNotFoundError:
        pass

if ext in ('.js', '.jsx', '.ts', '.tsx', '.mjs', '.cjs'):
    local_biome    = os.path.join(root, 'node_modules', '.bin', 'biome')
    local_prettier = os.path.join(root, 'node_modules', '.bin', 'prettier')
    biome_cfg      = os.path.exists(os.path.join(root, 'biome.json')) or \
                     os.path.exists(os.path.join(root, 'biome.jsonc'))

    if biome_cfg and os.path.exists(local_biome):
        run_formatter([local_biome, 'format', '--write', file_path])
    elif os.path.exists(local_prettier):
        run_formatter([local_prettier, '--write', file_path])
    elif biome_cfg and cmd_exists('biome'):
        run_formatter(['biome', 'format', '--write', file_path])
    elif cmd_exists('prettier'):
        run_formatter(['prettier', '--write', file_path])

elif ext == '.py':
    if cmd_exists('ruff'):
        run_formatter(['ruff', 'format', file_path])
    elif cmd_exists('black'):
        run_formatter(['black', '--quiet', file_path])

elif ext == '.go':
    if cmd_exists('gofmt'):
        run_formatter(['gofmt', '-w', file_path])

elif ext == '.rb':
    if cmd_exists('rubocop'):
        run_formatter(['rubocop', '--autocorrect-all', '--no-color', '--format', 'quiet', file_path])

PYFORMAT

exit 0
