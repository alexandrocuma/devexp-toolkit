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

# The trailing "x" keeps $(...) from trimming newlines that belong to the path.
file_path=$(echo "$input" | python3 -I -c \
    "import sys,json; d=json.load(sys.stdin); sys.stdout.write(str(d.get('tool_input',{}).get('file_path','')) + 'x')") || {
    echo "[devexp format-on-save] internal error -- could not read hook input, skipping. The interpreter's error is above." >&2
    exit 0
}
file_path=${file_path%x}

python3 -I - "$file_path" <<'PYFORMAT'
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

# A relative path that starts with '-' reaches a tool as an option, and ruff
# reads one that starts with '@' as an argument file, even after '--' (#121).
# Every tool reads a './' path as a path; an absolute path passes unchanged.
path_arg = file_path if os.path.isabs(file_path) else './' + file_path

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
        run_formatter([local_biome, 'format', '--write', path_arg])
    elif os.path.exists(local_prettier):
        run_formatter([local_prettier, '--write', path_arg])
    elif biome_cfg and cmd_exists('biome'):
        run_formatter(['biome', 'format', '--write', path_arg])
    elif cmd_exists('prettier'):
        run_formatter(['prettier', '--write', path_arg])

elif ext == '.py':
    if cmd_exists('ruff'):
        run_formatter(['ruff', 'format', path_arg])
    elif cmd_exists('black'):
        run_formatter(['black', '--quiet', path_arg])

elif ext == '.go':
    if cmd_exists('gofmt'):
        run_formatter(['gofmt', '-w', path_arg])

elif ext == '.rb':
    if cmd_exists('rubocop'):
        run_formatter(['rubocop', '--autocorrect-all', '--no-color', '--format', 'quiet', path_arg])

PYFORMAT

exit 0
