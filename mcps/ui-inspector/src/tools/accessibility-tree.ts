import { McpServer } from '@modelcontextprotocol/sdk/server/mcp.js';
import { z } from 'zod';
import { getPage } from '../browser.js';

export function registerAccessibilityTree(server: McpServer) {
  server.tool(
    'get_accessibility_tree',
    'Get the ARIA accessibility tree as YAML — reveals semantic structure, element roles, labels, heading hierarchy, and tab flow. Best signal for UX audit.',
    {
      selector: z
        .string()
        .optional()
        .describe('CSS selector to scope the tree to a specific subtree (default: full page body)'),
    },
    async ({ selector = 'body' }) => {
      const page = await getPage();
      const snapshot = await page.locator(selector).first().ariaSnapshot();
      return {
        content: [{ type: 'text', text: snapshot || '(empty accessibility tree)' }],
      };
    }
  );
}
