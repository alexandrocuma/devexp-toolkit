import { McpServer } from '@modelcontextprotocol/sdk/server/mcp.js';
import { z } from 'zod';
import { getPage } from '../browser.js';

export function registerNavigate(server: McpServer) {
  server.tool(
    'navigate',
    'Navigate the browser to a URL and wait for the page to load. Call this before any other inspection tool.',
    {
      url: z.string().describe('URL to navigate to'),
      wait_until: z
        .enum(['load', 'domcontentloaded', 'networkidle'])
        .optional()
        .describe('When to consider navigation done (default: networkidle)'),
    },
    async ({ url, wait_until = 'networkidle' }) => {
      const page = await getPage();
      await page.goto(url, { waitUntil: wait_until, timeout: 30000 });
      const title = await page.title();
      const currentUrl = page.url();
      return {
        content: [
          {
            type: 'text',
            text: `Navigated to: ${currentUrl}\nPage title: ${title}`,
          },
        ],
      };
    }
  );
}
