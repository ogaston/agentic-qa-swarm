# Customer run view verification

Implemented the approved presentation change in RunPage, Resultados and scoped editorial styles. Existing local changes, including bounded run-creation retries, were preserved.

## Customer experience

- Clear run heading and reference, readable verdicts and phases, and automatic progress messaging.
- Removed trace IDs, controller/reporter names, platform API route headings, workflow identifiers and implementation flags from visible UI. Known operational references in report prose are translated into customer language; customer API request examples are preserved.
- Report and flow errors use customer explanations instead of raw backend messages.
- Evidence locations are hidden from the page; a labeled button copies each original reference. Both successful copying and denied clipboard access have feedback and regression coverage.
- Balanced results/progress columns, consistent spacing and warm surfaces. Flow request tables become labeled stacked rows below 600px.

## Checks

From `web/dashboard`:

- `npm test`: 21 files, 242 tests passed.
- `npm run lint`: passed.
- `npm run build`: passed.
- `git diff --check -- src`: passed.

Browser script: `/tmp/run-customer-ui-browser.mjs`. Local build served at `http://127.0.0.1:18610`, using intercepted fixture API responses and a test login, not a live authenticated run. Browser host access was unavailable and the live API requires authentication; the actual run's report was not read.

At 1440, 390 and 375px: no document overflow, no visible tested internal labels, flow expansion works. Failed run alert verified. Zero page errors. Desktop and mobile screenshots visually inspected; mobile request fields remain readable without horizontal scrolling. Screenshots: `/tmp/run-customer-1440.png`, `/tmp/run-customer-390.png`, `/tmp/run-customer-375.png`.

The full repository whitespace check also reports the verbatim user's original trailing space retained in audit.md. Source changes pass the whitespace check.

No infrastructure deployment, merge or overall AI-DLC stage transition performed. Ready for review.

## Follow-up: lighter inner table

User's screenshot prompted a CSS-only refinement scoped to `.run-page .flow-content`. Removed the second frame, filled header and code/method backgrounds. Headers now use sentence case, regular sans-serif text and zero letter spacing. Preserved column labels, methods, request bodies and responsive stacked fields.

`npm run build` and `git diff --check -- src` pass. Re-ran `/tmp/run-customer-ui-browser.mjs`: 1440/390/375px without document overflow, expanded flow works, failed alert works, zero page errors. Updated desktop/mobile screenshots inspected. No new tests added for this reversible CSS-only refinement.
