import { getPage } from '../browser.js';
export function registerPageMetrics(server) {
    server.tool('get_page_metrics', 'Get page performance metrics: navigation timing, paint events (FP, FCP), resource count, and cumulative layout shift (CLS).', {}, async () => {
        const page = await getPage();
        const metrics = await page.evaluate(() => {
            const nav = performance.getEntriesByType('navigation')[0];
            const paint = performance.getEntriesByType('paint');
            const resources = performance.getEntriesByType('resource');
            return {
                ttfb_ms: nav ? Math.round(nav.responseStart - nav.requestStart) : null,
                dom_interactive_ms: nav ? Math.round(nav.domInteractive - nav.startTime) : null,
                dom_content_loaded_ms: nav ? Math.round(nav.domContentLoadedEventEnd - nav.startTime) : null,
                load_ms: nav ? Math.round(nav.loadEventEnd - nav.startTime) : null,
                first_paint_ms: Math.round(paint.find(e => e.name === 'first-paint')?.startTime ?? -1),
                first_contentful_paint_ms: Math.round(paint.find(e => e.name === 'first-contentful-paint')?.startTime ?? -1),
                resource_count: resources.length,
                total_transfer_kb: Math.round(resources.reduce((s, r) => s + r.transferSize, 0) / 1024),
            };
        });
        const cls = await page.evaluate(() => new Promise(resolve => {
            let value = 0;
            const obs = new PerformanceObserver(list => {
                for (const entry of list.getEntries()) {
                    value += entry.value ?? 0;
                }
            });
            try {
                obs.observe({ type: 'layout-shift', buffered: true });
            }
            catch { }
            setTimeout(() => { obs.disconnect(); resolve(Math.round(value * 1000) / 1000); }, 600);
        }));
        const formatted = { ...metrics, cumulative_layout_shift: cls };
        return {
            content: [{ type: 'text', text: JSON.stringify(formatted, null, 2) }],
        };
    });
}
