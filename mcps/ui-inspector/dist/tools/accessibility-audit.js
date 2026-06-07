import { z } from 'zod';
import { createRequire } from 'module';
import { getPage } from '../browser.js';
const require = createRequire(import.meta.url);
const axePath = require.resolve('axe-core');
export function registerAccessibilityAudit(server) {
    server.tool('run_accessibility_audit', 'Run an axe-core accessibility audit on the current page. Returns WCAG violations with impact level and fix guidance.', {
        include: z
            .string()
            .optional()
            .describe('CSS selector to scope the audit to a specific region (default: full page)'),
        impact: z
            .enum(['minor', 'moderate', 'serious', 'critical'])
            .optional()
            .describe('Minimum impact level to report (default: moderate)'),
    }, async ({ include, impact = 'moderate' }) => {
        const page = await getPage();
        await page.addScriptTag({ path: axePath });
        const IMPACT_ORDER = ['minor', 'moderate', 'serious', 'critical'];
        const minIndex = IMPACT_ORDER.indexOf(impact);
        const violations = await page.evaluate(async ({ includeSelector }) => {
            const opts = includeSelector ? { include: [[includeSelector]] } : {};
            const results = await window.axe.run(document, opts);
            return results.violations;
        }, { includeSelector: include });
        const filtered = violations.filter(v => IMPACT_ORDER.indexOf(v.impact) >= minIndex);
        if (filtered.length === 0) {
            return {
                content: [
                    {
                        type: 'text',
                        text: `No accessibility violations found at impact level "${impact}" or above.`,
                    },
                ],
            };
        }
        const report = filtered
            .map(v => {
            const nodeLines = v.nodes
                .slice(0, 3)
                .map(n => `    HTML: ${n.html.slice(0, 120)}\n    Fix:  ${n.failureSummary}`)
                .join('\n');
            return [
                `[${v.impact.toUpperCase()}] ${v.id}`,
                `  ${v.description}`,
                `  Help: ${v.helpUrl}`,
                `  Affected nodes (${v.nodes.length}):`,
                nodeLines,
            ].join('\n');
        })
            .join('\n\n');
        return {
            content: [
                {
                    type: 'text',
                    text: `${filtered.length} violation(s) found:\n\n${report}`,
                },
            ],
        };
    });
}
