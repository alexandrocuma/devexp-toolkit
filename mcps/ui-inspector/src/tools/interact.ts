import { McpServer } from '@modelcontextprotocol/sdk/server/mcp.js';
import { z } from 'zod';
import { getPage } from '../browser.js';

export function registerInteract(server: McpServer) {
  server.tool(
    'interact',
    'Interact with the current page to advance UI state, then call screenshot again to observe the result. Supports click, type, hover, scroll, and key press.',
    {
      action: z
        .enum(['click', 'type', 'hover', 'scroll', 'key'])
        .describe('Interaction type'),
      selector: z
        .string()
        .optional()
        .describe('CSS selector of the target element (required for click, type, hover)'),
      text: z
        .string()
        .optional()
        .describe('Text to type (required when action is "type")'),
      key: z
        .string()
        .optional()
        .describe('Key name to press, e.g. "Enter", "Tab", "Escape" (required when action is "key")'),
      scroll_x: z
        .number()
        .optional()
        .describe('Horizontal scroll delta in pixels (action "scroll")'),
      scroll_y: z
        .number()
        .optional()
        .describe('Vertical scroll delta in pixels (action "scroll", default: 300)'),
      wait_ms: z
        .number()
        .optional()
        .describe('Milliseconds to wait after the interaction before returning (default: 300)'),
    },
    async ({ action, selector, text, key, scroll_x = 0, scroll_y = 300, wait_ms = 300 }) => {
      const page = await getPage();

      switch (action) {
        case 'click': {
          if (!selector) return { content: [{ type: 'text', text: 'selector is required for click' }] };
          await page.locator(selector).first().click();
          break;
        }
        case 'type': {
          if (!selector) return { content: [{ type: 'text', text: 'selector is required for type' }] };
          if (text === undefined) return { content: [{ type: 'text', text: 'text is required for type' }] };
          await page.locator(selector).first().fill(text);
          break;
        }
        case 'hover': {
          if (!selector) return { content: [{ type: 'text', text: 'selector is required for hover' }] };
          await page.locator(selector).first().hover();
          break;
        }
        case 'scroll': {
          if (selector) {
            await page.locator(selector).first().evaluate(
              (el, { x, y }) => el.scrollBy(x, y),
              { x: scroll_x, y: scroll_y }
            );
          } else {
            await page.evaluate(({ x, y }) => window.scrollBy(x, y), { x: scroll_x, y: scroll_y });
          }
          break;
        }
        case 'key': {
          if (!key) return { content: [{ type: 'text', text: 'key is required for key action' }] };
          if (selector) {
            await page.locator(selector).first().press(key);
          } else {
            await page.keyboard.press(key);
          }
          break;
        }
      }

      await page.waitForTimeout(wait_ms);

      return {
        content: [{ type: 'text', text: `${action} completed. Call screenshot to observe the updated UI state.` }],
      };
    }
  );
}
