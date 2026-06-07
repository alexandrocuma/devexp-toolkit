import { chromium, Browser, BrowserContext, Page } from 'playwright-core';

const OBSCURA_CDP_URL = process.env.OBSCURA_CDP_URL ?? 'ws://127.0.0.1:9222';

let browser: Browser | null = null;
let context: BrowserContext | null = null;
let page: Page | null = null;

interface ConsoleEntry {
  type: string;
  text: string;
  timestamp: number;
}

const consoleLogs: ConsoleEntry[] = [];

export async function getPage(): Promise<Page> {
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

export function drainConsoleLogs(types?: string[]): ConsoleEntry[] {
  const logs = consoleLogs.splice(0);
  return types ? logs.filter(l => types.includes(l.type)) : logs;
}

export function peekConsoleLogs(types?: string[]): ConsoleEntry[] {
  return types ? consoleLogs.filter(l => types.includes(l.type)) : [...consoleLogs];
}

export async function closeBrowser(): Promise<void> {
  // Don't close Obscura — it's an external process. Just clean up our context.
  if (page && !page.isClosed()) await page.close().catch(() => {});
  if (context) await context.close().catch(() => {});
  browser = null;
  context = null;
  page = null;
}
