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
 * hooks.json is a JSON array, one entry per selected hook:
 *
 *   { "name": "secret-guard", "module": "secret-guard.js",
 *     "export": "secretGuard", "failClosed": true }
 *
 * `module` is a bare file name inside devexp/ — anything containing `/` or `..`
 * is refused. The mapping comes from the `opencode` block of each hook in
 * hooks/registry.json (`module`, `export`, `fail_closed`).
 *
 * Failure handling:
 *   - hooks.json missing, invalid or not an array → every tool call is blocked
 *     (fail closed) until devexp is re-installed.
 *   - a module that fails to import or initialise is skipped with a logged
 *     error; if its entry is failClosed (the security guards) every tool call
 *     is blocked with an internal-error message instead.
 *
 * Runtime:
 *   - `tool.execute.before` handlers run one after another in selection order;
 *     the first throw blocks the call.
 *   - opencode delivers file events only through the `event` hook, so the entry
 *     adapts `event` → `file.edited` and hands each module `{ file }`. Those
 *     handlers are advisory: their errors are logged, never rethrown.
 *
 * opencode calls every export of a plugin file, so this file exports exactly
 * one function.
 *
 * Tests: node hooks/opencode/devexp-plugin.test.js
 * @see https://opencode.ai/docs/plugins
 */

import { readFileSync } from 'fs';

export const DevExpPlugin = async (ctx) => {
  const dir = new URL('./devexp/', import.meta.url);

  let entries;
  try {
    entries = JSON.parse(readFileSync(new URL('hooks.json', dir), 'utf8'));
    if (!Array.isArray(entries)) throw new Error('hooks.json is not an array');
  } catch {
    return {
      'tool.execute.before': async () => {
        throw new Error(
          '[devexp] opencode plugin is misconfigured (devexp/hooks.json unreadable) — ' +
          're-run devexp install. Blocking to be safe.'
        );
      },
    };
  }

  const loaded = [];
  for (const entry of entries) {
    const name = entry?.name ?? '(unnamed)';
    try {
      const file = entry?.module;
      if (typeof file !== 'string' || !file || file.includes('/') || file.includes('..')) {
        throw new Error(`module ${JSON.stringify(file)} is not a bare file name inside devexp/`);
      }
      const mod = await import(new URL(file, dir).href);
      const factory = mod[entry.export];
      if (typeof factory !== 'function') throw new Error(`export ${entry.export} is not a function`);
      loaded.push({ name, hooks: (await factory(ctx)) ?? {} });
    } catch (err) {
      if (entry?.failClosed) {
        loaded.push({
          name,
          hooks: {
            'tool.execute.before': async () => {
              throw new Error(
                `[devexp ${name}] internal error — the guard failed to load, so it did not run. ` +
                `Blocking to be safe: ${err?.message ?? err}`
              );
            },
          },
        });
      } else {
        console.error(`[devexp ${name}] failed to load; skipped: ${err?.message ?? err}`);
      }
    }
  }

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
    // opencode dispatches `event` with `void`, so nothing here may reject.
    event: async (input) => {
      const event = input?.event;
      if (event?.type !== 'file.edited') return;
      const payload = { file: event.properties?.file };
      for (const m of loaded) {
        if (!m.hooks['file.edited']) continue;
        try {
          await m.hooks['file.edited'](payload);
        } catch (e) {
          console.error(`[devexp ${m.name}] file.edited failed: ${e?.message ?? e}`);
        }
      }
    },
  };
};
