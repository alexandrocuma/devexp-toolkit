import { chromium } from 'playwright-core';
const OBSCURA_CDP_URL = process.env.OBSCURA_CDP_URL ?? 'ws://127.0.0.1:9222';
let browser = null;
let context = null;
let page = null;
const consoleLogs = [];
export async function getPage() {
    if (!browser) {
        browser = await chromium.connectOverCDP(OBSCURA_CDP_URL);
    }
    if (!context) {
        context = await browser.newContext();
    }
    if (!page || page.isClosed()) {
        page = await context.newPage();
        page.on('console', msg => {
            consoleLogs.push({ type: msg.type(), text: msg.text(), timestamp: Date.now() });
        });
        page.on('pageerror', err => {
            consoleLogs.push({ type: 'error', text: err.message, timestamp: Date.now() });
        });
    }
    return page;
}
export function drainConsoleLogs(types) {
    const logs = consoleLogs.splice(0);
    return types ? logs.filter(l => types.includes(l.type)) : logs;
}
export function peekConsoleLogs(types) {
    return types ? consoleLogs.filter(l => types.includes(l.type)) : [...consoleLogs];
}
export async function closeBrowser() {
    // Don't close Obscura — it's an external process. Just clean up our context.
    if (page && !page.isClosed())
        await page.close().catch(() => { });
    if (context)
        await context.close().catch(() => { });
    browser = null;
    context = null;
    page = null;
}
