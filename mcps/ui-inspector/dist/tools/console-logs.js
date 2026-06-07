import { z } from 'zod';
import { drainConsoleLogs, peekConsoleLogs } from '../browser.js';
export function registerConsoleLogs(server) {
    server.tool('get_console_logs', 'Get browser console messages (errors, warnings, logs) captured since the browser was opened or last drained. Reveals silent JS failures that break UX.', {
        drain: z
            .boolean()
            .optional()
            .describe('Clear the log buffer after reading (default: true)'),
        types: z
            .array(z.string())
            .optional()
            .describe('Filter by type: error, warning, log, info, debug (default: all)'),
    }, async ({ drain = true, types }) => {
        const logs = drain ? drainConsoleLogs(types) : peekConsoleLogs(types);
        if (logs.length === 0) {
            return {
                content: [{ type: 'text', text: '(no console messages captured)' }],
            };
        }
        const formatted = logs
            .map(l => `[${l.type.toUpperCase()}] ${l.text}`)
            .join('\n');
        return {
            content: [{ type: 'text', text: formatted }],
        };
    });
}
