#!/usr/bin/env bash
# devexp hook: lint-on-save
# Event: PostToolUse | Matcher: Write|Edit
# Runs the project linter on edited source files. Advisory only — cannot block.
#
# Linter priority per language:
#   JS/TS:  local biome (if biome.json) > local eslint > global biome > global eslint
#   Python: ruff > flake8
#   Go:     go vet
#   Ruby:   rubocop
#
# Silent if no linter is installed. Times out after 10s.

set -euo pipefail

input=$(cat)

# The trailing "x" keeps $(...) from trimming newlines that belong to the path.
file_path=$(echo "$input" | python3 -I -c \
    "import sys,json; d=json.load(sys.stdin); sys.stdout.write(str(d.get('tool_input',{}).get('file_path','')) + 'x')") || {
    echo "[devexp lint-on-save] internal error -- could not read hook input, skipping. The interpreter's error is above." >&2
    exit 0
}
file_path=${file_path%x}

python3 -I - "$file_path" <<'PYLINT'
import sys, os, shutil, subprocess

file_path = sys.argv[1]
if not file_path or not os.path.exists(file_path):
    sys.exit(0)

ext = os.path.splitext(file_path)[1].lower()

LINT_EXTS = {'.js', '.jsx', '.ts', '.tsx', '.mjs', '.cjs', '.py', '.go', '.rb'}
if ext not in LINT_EXTS:
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

def run_linter(cmd, cwd=None):
    try:
        r = subprocess.run(cmd, capture_output=True, text=True, timeout=10, cwd=cwd or root)
        output = (r.stdout + r.stderr).strip()
        if output:
            print(f'[devexp lint-on-save] {os.path.basename(cmd[0])}:', file=sys.stderr)
            print(output, file=sys.stderr)
    except subprocess.TimeoutExpired:
        print(f'[devexp lint-on-save] {os.path.basename(cmd[0])} timed out (>10s)', file=sys.stderr)
    except FileNotFoundError:
        pass

if ext in ('.js', '.jsx', '.ts', '.tsx', '.mjs', '.cjs'):
    local_biome  = os.path.join(root, 'node_modules', '.bin', 'biome')
    local_eslint = os.path.join(root, 'node_modules', '.bin', 'eslint')
    biome_cfg    = os.path.exists(os.path.join(root, 'biome.json')) or \
                   os.path.exists(os.path.join(root, 'biome.jsonc'))

    if biome_cfg and os.path.exists(local_biome):
        run_linter([local_biome, 'lint', path_arg])
    elif os.path.exists(local_eslint):
        run_linter([local_eslint, '--max-warnings=0', '--no-warn-ignored', path_arg])
    elif biome_cfg and cmd_exists('biome'):
        run_linter(['biome', 'lint', path_arg])
    elif cmd_exists('eslint'):
        run_linter(['eslint', '--max-warnings=0', path_arg])

elif ext == '.py':
    if cmd_exists('ruff'):
        run_linter(['ruff', 'check', path_arg])
    elif cmd_exists('flake8'):
        run_linter(['flake8', path_arg])

elif ext == '.go':
    if cmd_exists('go'):
        rel_dir = os.path.relpath(os.path.dirname(os.path.abspath(file_path)), root)
        pkg = f'./{rel_dir}' if rel_dir != '.' else './...'
        run_linter(['go', 'vet', pkg], cwd=root)

elif ext == '.rb':
    if cmd_exists('rubocop'):
        run_linter(['rubocop', '--no-color', '--format', 'simple', path_arg])
PYLINT

exit 0
