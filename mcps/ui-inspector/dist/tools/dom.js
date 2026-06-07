import { z } from 'zod';
import { getPage } from '../browser.js';
const MAX_CHARS = 60000;
export function registerDom(server) {
    server.tool('get_dom', 'Get the HTML DOM structure of the current page or a scoped element. Useful for inspecting component nesting and markup quality.', {
        selector: z
            .string()
            .optional()
            .describe('CSS selector to scope (default: body)'),
        strip_scripts: z
            .boolean()
            .optional()
            .describe('Remove <script> and <style> tags for cleaner output (default: true)'),
    }, async ({ selector = 'body', strip_scripts = true }) => {
        const page = await getPage();
        const html = await page.$eval(selector, (el, stripScripts) => {
            const clone = el.cloneNode(true);
            if (stripScripts) {
                clone.querySelectorAll('script, style').forEach(n => n.remove());
            }
            clone.querySelectorAll('[style]').forEach(n => n.removeAttribute('style'));
            return clone.outerHTML;
        }, strip_scripts);
        const truncated = html.length > MAX_CHARS
            ? html.slice(0, MAX_CHARS) + '\n\n... [truncated — use a tighter selector]'
            : html;
        return {
            content: [{ type: 'text', text: truncated }],
        };
    });
}
