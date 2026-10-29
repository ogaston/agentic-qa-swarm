# Run creation race — verification

Verified 2026-10-29T00:40:43Z (2026-10-28 in America/Bogota).

## Result

Run detail tolerates HTTP 404 before the first successful read, sharing the existing five-consecutive-failure budget with other transient failures. Requests remain three seconds apart, with no overlapping requests. While waiting it shows an accessible status message instead of a permanent not-found alert. Five consecutive initial 404s stop polling and expose the existing retry action; the header reads “No encontrada”. Retrying starts a new budget. A 404 after a successful read still stops polling immediately. The backend and generic polling hook are unchanged.

Only application files changed for this correction:

- `web/dashboard/src/pages/Run/RunPage.tsx`
- `web/dashboard/src/pages/Run/RunPage.test.tsx`

Existing edits to Inbox, Badge, component tests and stylesheet files were preserved.

## Evidence

| Command, from web/dashboard | Result |
| --- | --- |
| `npx vitest run src/pages/Run/RunPage.test.tsx` before implementation | 5 failed, 12 passed; regression demonstrated |
| `npx vitest run src/pages/Run/RunPage.test.tsx` after implementation | 17 passed |
| `npm test` | 241 passed across 21 files |
| `npm run lint` | Passed |
| `npm run check:api` | Passed; generated API types match contract |
| `npm run build` | TypeScript check and Vite production build passed |
| `git diff --check` from repository root | Passed |

Regression coverage includes initial 404 recovery through confirmed/running/done without reloading, five-failure exhaustion, manual retry followed by another 404 and eventual recovery, cancellation on unmount, route-change cancellation with a fresh retry budget, and fatal 404 after a successful lookup. Existing visibility and terminal-state tests also pass.

## Limits and constraints

The reported live run was not queried and no infrastructure was changed. Verification uses the real React page and MSW responses with fake timers; it does not claim a live end-to-end run. A run still unavailable after five consecutive failures needs a manual retry. No authentication, authorization, confirmation, contract, or deployment changes were needed. AUTONOMIA-01 is satisfied by leaving infrastructure untouched; AUTONOMIA-02 is satisfied by the commands above.
