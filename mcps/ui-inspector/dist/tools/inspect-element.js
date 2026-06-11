import { z } from 'zod';
import { getPage } from '../browser.js';
const DEFAULT_PROPS = [
    'display', 'position', 'width', 'height', 'min-width', 'max-width',
    'min-height', 'max-height', 'margin', 'margin-top', 'margin-right',
    'margin-bottom', 'margin-left', 'padding', 'padding-top', 'padding-right',
    'padding-bottom', 'padding-left', 'color', 'background-color', 'font-size',
    'font-weight', 'font-family', 'line-height', 'text-align', 'overflow',
    'overflow-x', 'overflow-y', 'z-index', 'opacity', 'border', 'border-radius',
    'flex', 'flex-direction', 'align-items', 'justify-content', 'gap',
    'grid-template-columns', 'grid-template-rows', 'cursor', 'visibility',
];
export function registerInspectElement(server) {
    server.tool('inspect_element', 'Get computed CSS styles and bounding rect for a CSS selector. Use to diagnose spacing, sizing, overflow, and layout issues.', {
        selector: z.string().describe('CSS selector of the element to inspect'),
        properties: z
            .array(z.string())
            .optional()
            .describe('Specific CSS property names to fetch (default: common layout + visual properties)'),
    }, async ({ selector, properties }) => {
        const page = await getPage();
        const el = await page.$(selector);
        if (!el) {
            return {
                content: [{ type: 'text', text: `No element found for selector: ${selector}` }],
            };
        }
        const result = await page.$eval(selector, (element, props) => {
            const styles = window.getComputedStyle(element);
            const rect = element.getBoundingClientRect();
            const computed = {};
            for (const prop of props) {
                computed[prop] = styles.getPropertyValue(prop).trim();
            }
            return {
                tag: element.tagName.toLowerCase(),
                id: element.id || undefined,
                classes: element.className || undefined,
                bounding_rect: {
                    top: Math.round(rect.top),
                    left: Math.round(rect.left),
                    width: Math.round(rect.width),
                    height: Math.round(rect.height),
                    visible: rect.width > 0 && rect.height > 0,
                },
                computed_styles: computed,
            };
        }, properties ?? DEFAULT_PROPS);
        return {
            content: [{ type: 'text', text: JSON.stringify(result, null, 2) }],
        };
    });
}
