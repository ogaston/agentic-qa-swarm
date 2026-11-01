# Login persistence and Inbox updates — Verification

User approval: “Implement the plan.” Completed on 2026-10-29 UTC (2026-10-28 America/Bogota).

## Delivered behavior

- Successful login persists only version, token and expiration under `aqs.session.v1`. A reload validates local data and revalidates principal, role and expiration through `/api/auth/session` before mounting protected routes. Invalid storage, restore failures, expiration and authenticated 401 fail closed. Logout clears storage even on server failure. Blocked storage preserves an in-memory login.
- Inbox loads `/api/notifications` without a state filter. Its complete dataset supplies all KPI/tab totals, and active tabs filter table rows locally. Existing refresh and action refreshes reload the dataset with stale-response protection. The card header keeps the tabs and refresh available in empty, error and loading states.
- The UI API projects existing `notify.created.occurred_at` into REST `created_at`, preserving it across confirmation and event replay. OpenAPI and generated dashboard types include the additive optional field for compatibility with older responses; no event schema or persistence migration is required. Missing/invalid dates show a dash. Valid dates show Spanish relative age, an exact local timestamp tooltip and semantic time markup; a minute timer updates text without polling the API.
- Table artifacts show kind plus seven trailing reference characters. Full references appear on hover/focus, support Escape, and render through a portal to avoid clipping inside horizontally scrollable tables.
- Both modal steps link repositories and commits to GitHub, validate destination components, open a new tab with `noopener noreferrer`, and include external arrows and accessible new-tab descriptions. Links participate in the existing modal focus trap.

## Commands and results

| Check | Result |
| --- | --- |
| `npm test --prefix web/dashboard` | 270 tests passed across 23 files |
| `npm run lint --prefix web/dashboard` | Passed; invoked through `rtk bypass` because the output wrapper could not parse ESLint's empty success output |
| `npm run build --prefix web/dashboard` | Passed TypeScript and production Vite build |
| `npm run check:api --prefix web/dashboard` | Generated API types match OpenAPI |
| `cd services/ui-api && GOTOOLCHAIN=auto go test ./...` | All six packages passed, including notification ordering/idempotence and JSON round-trip properties |
| `git diff --check -- web/dashboard services/ui-api contracts aidlc-docs/construction/plans/login-inbox-updates-code-generation-plan.md` | Passed |
| `node /tmp/login-inbox-browser.mjs` | Production build, fixture API, strict CSP: 1440/390/375px passed; zero page/console errors or page overflow |

The environment defaults to Go 1.26.9 with `GOTOOLCHAIN=local`; this module requires Go 1.26.10. The successful Go test command enabled automatic selection without changing project files.

Dashboard output retains three pre-existing MSW warnings for the run-detail GET reached by confirmation-navigation tests; all assertions pass. Browser verification intercepted every API request and reported zero errors. No live identity server or shared environment was changed.

## Browser evidence

The fixture-backed browser script verified actual form login, localStorage persistence, full reload restoration, logout cleanup, stable global KPIs across tab changes, relative timestamps, tooltip visibility within viewport boundaries, link destinations in both modal steps, and Tab/Escape modal behavior. Two total notification requests occurred: initial load and full reload; tab changes performed no API calls.

Screenshots inspected: `/tmp/inbox-updates-1440.png` and `/tmp/inbox-modal-375-0.png`. Additional screenshots exist for Inbox and both modal steps at 1440, 390 and 375px. Browser output: `/tmp/login-inbox-browser.log`.

## Constraints and compatibility

LocalStorage persistence explicitly supersedes the previous human-approved memory-only policy. Persisted bearer tokens are readable by scripts on the same origin; server validation, expiry cleanup and the existing strict CSP remain in place. No passwords or OTPs enter storage, DOM or logs. Browser tests run with `script-src 'self'` and `style-src 'self'`; tooltip positioning succeeds without relaxing CSP.

Applicable controls: authentication isolation/server validation and safe failure paths (SECURITY-11/12/13/15), no credential logging (SECURITY-03), unchanged strict CSP (SECURITY-04), fixed GitHub origins and escaped rendered values (SECURITY-05). Infrastructure/storage-service provisioning and deployment controls are outside this patch. Existing notification ordering/idempotence and JSON round-trip properties remain verified under partial PBT enforcement. AUTONOMIA-01: no infrastructure change. AUTONOMIA-02: acceptance behavior verified by the commands above.

Other concurrent changes in demo scripts and documentation were left intact. Overall AI-DLC stage remains CONSTRUCTION Code Generation, ready for review.
