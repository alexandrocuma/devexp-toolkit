#!/usr/bin/env bash
# devexp hook: test-on-save
# Event: PostToolUse | Matcher: Write|Edit
# Runs the test file associated with an edited source file. Advisory only — cannot block.
#
# Test discovery per language:
#   JS/TS:  <name>.test.{ts,js,tsx,jsx} or <name>.spec.{ts,js} in same dir or __tests__/
#   Go:     go test ./<package> (package containing the edited file)
#   Python: test_<name>.py or <name>_test.py in same dir or tests/
#   Ruby:   spec/<path>/<name>_spec.rb
#
# Silent if no matching test file is found. Times out after 20s.

set -euo pipefail

input=$(cat)

file_path=$(echo "$input" | python3 -c \
    "import sys,json; d=json.load(sys.stdin); print(d.get('tool_input',{}).get('file_path',''))" \
    2>/dev/null || echo "")

python3 - "$file_path" <<'PYTEST'
import sys, os, shutil, subprocess, glob

file_path = sys.argv[1]
if not file_path or not os.path.exists(file_path):
    sys.exit(0)

ext = os.path.splitext(file_path)[1].lower()
basename = os.path.splitext(os.path.basename(file_path))[0]
file_dir  = os.path.dirname(os.path.abspath(file_path))

# Skip test files themselves — we only trigger on source files
TEST_MARKERS = ('.test.', '.spec.', '_test.', 'test_')
if any(m in os.path.basename(file_path) for m in TEST_MARKERS) or \
   basename.startswith('test_') or basename.endswith('_test') or \
   basename.endswith('_spec'):
    sys.exit(0)

SOURCE_EXTS = {'.js', '.jsx', '.ts', '.tsx', '.mjs', '.cjs', '.py', '.go', '.rb'}
if ext not in SOURCE_EXTS:
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

def cmd_exists(cmd):
    return shutil.which(cmd) is not None

def run_tests(cmd, cwd=None):
    label = os.path.basename(cmd[0])
    try:
        r = subprocess.run(cmd, capture_output=True, text=True, timeout=20, cwd=cwd or root)
        output = (r.stdout + r.stderr).strip()
        status = 'PASS' if r.returncode == 0 else 'FAIL'
        print(f'[devexp test-on-save] {label} [{status}]', file=sys.stderr)
        if output:
            print(output, file=sys.stderr)
    except subprocess.TimeoutExpired:
        print(f'[devexp test-on-save] {label} timed out (>20s) — run tests manually', file=sys.stderr)
    except FileNotFoundError:
        pass

root = find_root(file_path)

# ── JS / TS ─────────────────────────────────────────────────────────────────
if ext in ('.js', '.jsx', '.ts', '.tsx', '.mjs', '.cjs'):
    # Candidate test file names
    candidates = []
    for test_ext in ('.test.ts', '.test.tsx', '.test.js', '.test.jsx',
                     '.spec.ts', '.spec.tsx', '.spec.js', '.spec.jsx'):
        candidates.append(os.path.join(file_dir, basename + test_ext))
        candidates.append(os.path.join(file_dir, '__tests__', basename + test_ext))

    test_file = next((c for c in candidates if os.path.exists(c)), None)
    if not test_file:
        sys.exit(0)  # No test file found — skip silently

    local_jest    = os.path.join(root, 'node_modules', '.bin', 'jest')
    local_vitest  = os.path.join(root, 'node_modules', '.bin', 'vitest')

    if os.path.exists(local_vitest):
        run_tests([local_vitest, 'run', test_file], cwd=root)
    elif os.path.exists(local_jest):
        run_tests([local_jest, '--testPathPattern', os.path.relpath(test_file, root),
                   '--passWithNoTests', '--no-coverage'], cwd=root)
    elif cmd_exists('vitest'):
        run_tests(['vitest', 'run', test_file], cwd=root)
    elif cmd_exists('jest'):
        run_tests(['jest', '--testPathPattern', os.path.relpath(test_file, root),
                   '--passWithNoTests', '--no-coverage'], cwd=root)

# ── Go ───────────────────────────────────────────────────────────────────────
elif ext == '.go':
    if not cmd_exists('go'):
        sys.exit(0)
    # Run tests for the package that contains this file
    rel_dir = os.path.relpath(file_dir, root)
    pkg = f'./{rel_dir}' if rel_dir != '.' else './...'
    run_tests(['go', 'test', '-timeout', '20s', pkg], cwd=root)

# ── Python ───────────────────────────────────────────────────────────────────
elif ext == '.py':
    candidates = [
        os.path.join(file_dir, f'test_{basename}.py'),
        os.path.join(file_dir, f'{basename}_test.py'),
        os.path.join(root, 'tests', f'test_{basename}.py'),
        os.path.join(root, 'tests', f'{basename}_test.py'),
    ]
    test_file = next((c for c in candidates if os.path.exists(c)), None)
    if not test_file:
        sys.exit(0)

    if cmd_exists('pytest'):
        run_tests(['pytest', test_file, '-x', '-q'], cwd=root)

# ── Ruby ─────────────────────────────────────────────────────────────────────
elif ext == '.rb':
    # Map lib/<path>/<name>.rb → spec/<path>/<name>_spec.rb
    rel = os.path.relpath(file_path, root)
    spec_rel = rel.replace('lib/', 'spec/', 1).replace('.rb', '_spec.rb')
    spec_path = os.path.join(root, spec_rel)
    if not os.path.exists(spec_path):
        sys.exit(0)

    if cmd_exists('rspec'):
        run_tests(['rspec', spec_path, '--format', 'progress'], cwd=root)

PYTEST

exit 0
