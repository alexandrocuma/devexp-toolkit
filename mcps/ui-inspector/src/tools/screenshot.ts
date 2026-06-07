import { McpServer } from '@modelcontextprotocol/sdk/server/mcp.js';
import { z } from 'zod';
import { getPage } from '../browser.js';

export function registerScreenshot(server: McpServer) {
  server.tool(
    'screenshot',
    'Take a screenshot of the current page or a specific element. Returns a visual image for layout, spacing, and design review.',
    {
      full_page: z
        .boolean()
        .optional()
        .describe('Capture the full scrollable page height (default: true)'),
      selector: z
        .string()
        .optional()
        .describe('CSS selector to screenshot a specific element only'),
      width: z
        .number()
        .optional()
        .describe('Viewport width in pixels (default: 1280)'),
      height: z
        .number()
        .optional()
        .describe('Viewport height in pixels (default: 800)'),
    },
    async ({ full_page = true, selector, width = 1280, height = 800 }) => {
      const page = await getPage();
      await page.setViewportSize({ width, height });

      let buffer: Buffer;
      if (selector) {
        const el = page.locator(selector).first();
        buffer = await el.screenshot();
      } else {
        buffer = await page.screenshot({ fullPage: full_page });
      }

      return {
        content: [
          {
            type: 'image',
            data: buffer.toString('base64'),
            mimeType: 'image/png',
          },
        ],
      };
    }
  );
}
