/**
 * devexp-plugin.js — entry point for devexp opencode hooks
 *
 * Composes the hook modules listed in the installed selection file instead of
 * importing them statically, so it honours hook selection and one broken
 * module cannot take the others down. Installed layout:
 *
 *   <plugins>/devexp.js              this file
 *   <plugins>/devexp/hooks.json      the selection, in registry order
 *   <plugins>/devexp/<module>.js     the hook modules, plus utils.js and package.json
 *
 * hooks.json is a non-empty JSON array, one entry per selected hook, with
 * exactly these keys:
 *
 *   { "name": "secret-guard", "module": "secret-guard.js",
 *     "export": "secretGuard", "failClosed": true }
 *
 * `module` must be a bare `.js` file name (`^[A-Za-z0-9][A-Za-z0-9._-]*\.js$`)
 * that resolves to a file: URL inside devexp/ — anything else is refused. The
 * mapping comes from the `opencode` block of each hook in hooks/registry.json
 * (`module`, `export`, `fail_closed`).
 *
 * Failure handling — a security guard must never fail open silently:
 *   - hooks.json missing, invalid, not an array, or empty → every tool call is
 *     blocked until devexp is re-installed (the installer never writes an empty
 *     selection; with every hook disabled it installs no plugin at all).
 *   - an entry that is not an object with a string `name` → blocking stub.
 *   - a module that fails to import or initialise is skipped with a logged
 *     error, unless the entry is fail-closed — `failClosed: true`, the registry
 *     spelling `fail_closed: true`, or one of the built-in security guards —
 *     in which case every tool call is blocked with an internal-error message.
 *
 * Runtime:
 *   - `tool.execute.before` handlers run one after another in selection order;
 *     the first throw blocks the call.
 *   - opencode delivers file events only through the `event` hook, so the entry
 *     adapts `event` → `file.edited` and hands each module `{ file }`. The
 *     handlers are queued and run after `event` returns (one edit at a time, in
 *     selection order), so opencode's event publisher is never held while a
 *     linter, formatter or test runner works. They are advisory: their errors
 *     are logged, never rethrown.
 *
 * opencode calls every export of a plugin file, so this file exports exactly
 * one function.
 *
 * Tests: node hooks/opencode/devexp-plugin.test.js
 * @see https://opencode.ai/docs/plugins
 */

import { readFileSync } from 'fs';

// Defense in depth. The registry marks these guards `fail_closed` and the
// installer carries that into hooks.json as `failClosed`, so this set repeats
// the registry on purpose: a guard must not fail open because that one flag
// was dropped or misspelled between the two.
const SECURITY_GUARDS = new Set(['secret-guard', 'secret-in-write-guard', 'dangerous-cmd-guard']);

const MODULE_FILE = /^[A-Za-z0-9][A-Za-z0-9._-]*\.js$/;

const blockAll = (message) => ({
  'tool.execute.before': async () => {
    throw new Error(message);
  },
});

const misconfigured = (why) =>
  `[devexp] opencode plugin is misconfigured (${why}) — re-run devexp install. Blocking to be safe.`;

export const DevExpPlugin = async (ctx) => {
  const dir = new URL('./devexp/', import.meta.url);

  let entries;
  try {
    entries = JSON.parse(readFileSync(new URL('hooks.json', dir), 'utf8'));
  } catch {
    entries = null;
  }
  if (!Array.isArray(entries) || entries.length === 0) {
    return blockAll(misconfigured('devexp/hooks.json unreadable'));
  }

  const loaded = [];
  for (const [i, entry] of entries.entries()) {
    const isEntry = entry !== null && typeof entry === 'object' && !Array.isArray(entry) &&
      typeof entry.name === 'string' && entry.name !== '';
    if (!isEntry) {
      loaded.push({ name: `entry ${i}`, hooks: blockAll(misconfigured(`devexp/hooks.json entry ${i} is not a hook entry`)) });
      continue;
    }

    const { name } = entry;
    const failClosed = entry.failClosed === true || entry.fail_closed === true || SECURITY_GUARDS.has(name);
    try {
      const file = entry.module;
      if (typeof file !== 'string' || !MODULE_FILE.test(file)) {
        throw new Error(`module ${JSON.stringify(file)} is not a bare file name inside devexp/`);
      }
      const url = new URL(file, dir);
      if (url.protocol !== 'file:' || !url.href.startsWith(dir.href)) {
        throw new Error(`module ${JSON.stringify(file)} resolves outside devexp/`);
      }
      const mod = await import(url.href);
      const factory = mod[entry.export];
      if (typeof factory !== 'function') throw new Error(`export ${entry.export} is not a function`);
      loaded.push({ name, hooks: (await factory(ctx)) ?? {} });
    } catch (err) {
      if (failClosed) {
        loaded.push({
          name,
          hooks: blockAll(
            `[devexp ${name}] internal error — the guard failed to load, so it did not run. ` +
            `Blocking to be safe: ${err?.message ?? err}`
          ),
        });
      } else {
        console.error(`[devexp ${name}] failed to load; skipped: ${err?.message ?? err}`);
      }
    }
  }

  // Edits are handled one at a time, in order, off opencode's publish path.
  let fileEdits = Promise.resolve();

  return {
    // Run all tool.execute.before handlers in sequence — first throw wins
    'tool.execute.before': async (input, output) => {
      for (const m of loaded) {
        if (m.hooks['tool.execute.before']) {
          await m.hooks['tool.execute.before'](input, output);
        }
      }
    },

    // Adapt opencode's bus events to the modules' file.edited handlers.
    // opencode dispatches `event` with `void`, so nothing here may reject, and
    // the handlers spawn tools, so they are queued rather than awaited here.
    event: async (input) => {
      const event = input?.event;
      if (event?.type !== 'file.edited') return;
      const payload = { file: event.properties?.file };
      const handlers = loaded.filter((m) => typeof m.hooks['file.edited'] === 'function');
      if (handlers.length === 0) return;

      fileEdits = fileEdits
        .then(() => new Promise((resolve) => setImmediate(resolve)))
        .then(async () => {
          for (const m of handlers) {
            try {
              await m.hooks['file.edited'](payload);
            } catch (e) {
              console.error(`[devexp ${m.name}] file.edited failed: ${e?.message ?? e}`);
            }
          }
        })
        .catch(() => {});
    },
  };
};
