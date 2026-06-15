/**
 * Tests for dangerous-cmd-guard.js — mirrors hooks/claude-code/dangerous-cmd-guard.test.sh.
 * Run: node hooks/opencode/dangerous-cmd-guard.test.js
 */
import { BLOCK_PATTERNS } from './dangerous-cmd-guard.js';

const blocked = (cmd) => BLOCK_PATTERNS.some(({ re }) => re.test(cmd));

const BLOCK = [
  'rm -f /tmp/*',
  'rm -rf /tmp',
  'rm -rf /tmp/',
  'rm -f /tmp/*"$ticket"*',
  'rm -rf ~/.claude',
  'rm -rf $HOME/.claude',
  'rm -f ~/.claude/agent-memory/grooming-agent/sessions/*"$id"*',
  'rm -f ~/.claude/agent-memory/*',
  'rm -rf /',
  'git push --force',
  'git push origin main -f',
  'git push --force-with-lease',
  "rm -rf '/tmp'/*",
  'rm -rf "/tmp"/*',
  'rm -rf "$HOME"/.claude',
  'git commit -m x && git push --force',
];

const ALLOW = [
  'rm -f /tmp/.deliver-PAY-123-*',
  'rm -f /tmp/.recently_changed.txt',
  'rm -f ~/.claude/agent-memory/grooming-agent/plans/PAY-123.md',
  'rm -f ~/.claude/agent-memory/grooming-agent/sessions/PAY-123-*',
  'git push origin main',
  'git commit -m "fix -f false positive" ; git push',
  'git push && rm -f /tmp/.deliver-X-1',
  'rm -rf node_modules',
  'rm -rf ./dist',
];

let fail = 0;
for (const c of BLOCK) if (!blocked(c)) { console.log('FAIL want block:', c); fail++; }
for (const c of ALLOW) if (blocked(c)) { console.log('FAIL want allow:', c); fail++; }

console.log(`${BLOCK.length + ALLOW.length - fail} passed, ${fail} failed`);
process.exit(fail === 0 ? 0 : 1);
