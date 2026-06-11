import { McpServer } from '@modelcontextprotocol/sdk/server/mcp.js';
import { StdioServerTransport } from '@modelcontextprotocol/sdk/server/stdio.js';
import { closeBrowser } from './browser.js';
import { registerNavigate } from './tools/navigate.js';
import { registerScreenshot } from './tools/screenshot.js';
import { registerAccessibilityTree } from './tools/accessibility-tree.js';
import { registerDom } from './tools/dom.js';
import { registerConsoleLogs } from './tools/console-logs.js';
import { registerInspectElement } from './tools/inspect-element.js';
import { registerAccessibilityAudit } from './tools/accessibility-audit.js';
import { registerPageMetrics } from './tools/page-metrics.js';
const server = new McpServer({
    name: 'ui-inspector',
    version: '0.1.0',
});
registerNavigate(server);
registerScreenshot(server);
registerAccessibilityTree(server);
registerDom(server);
registerConsoleLogs(server);
registerInspectElement(server);
registerAccessibilityAudit(server);
registerPageMetrics(server);
async function shutdown() {
    await closeBrowser();
    process.exit(0);
}
process.on('SIGINT', shutdown);
process.on('SIGTERM', shutdown);
const transport = new StdioServerTransport();
await server.connect(transport);
