# Live tests in run detail — verification

Implemented the approved plan in the existing run view, preserving pre-existing local edits.

## Behavior

- The current timeline phase displays a spinner instead of “actual”, with an accessible “En curso” label and `aria-current="step"`. Reduced motion disables rotation.
- During active runs, test flows appear before the pending report. Individual test flows expand when execution begins, including direct navigation to a running run and late plan arrival. Users can still collapse them; run polling does not reopen them.
- A not-yet-ready plan retries every three seconds, up to five attempts per budget. Entering execution immediately refreshes a pending plan and renews the budget, including when rehearsal exhausted it. Success and permission errors stop retries. Terminal transitions and unmount cancel scheduled retries. Manual retry is available after exhaustion and for terminal runs.
- Test steps display the existing plan; per-test execution results are not supplied by this API and are not fabricated.

## Checks

From `web/dashboard`:

- `npm test -- --reporter=dot`: 21 files, 250 tests passed.
- `npm run lint`: passed.
- `npm run build`: passed.
- `git diff --check -- web/dashboard/src` from the repository root: passed.

Eight added regression tests cover direct running entry and ordering, expansion on transition, delayed readiness recovery, bounded retries/manual recovery, unmount cleanup, terminal cleanup, permission failure, and renewed readiness checks when execution starts after an exhausted rehearsal budget. Existing timeline fidelity assertions now check the spinner and accessible state.

The full suite emits existing React Router future warnings and Inbox navigation fixture warnings, but all tests pass. An initial new test query matched two table headers; it was corrected to assert both. Lint caught one non-null assertion in the new test, which was removed.

## Browser verification

Final production build served locally at `http://127.0.0.1:18611`; `/tmp/run-live-tests-browser.mjs` used Playwright with fixture API interception and a test login. Verified route `/runs/run-7e56e11a98a8bce7` with a simulated running state and an initial `plan_not_ready` response, followed by a successful plan response.

- 1440, 390 and 375px: expanded test steps visible, plan first, active spinner, no horizontal document overflow.
- Automatic recovery on the second plan request without reload; no repeated plan requests after success.
- Manual flow collapse persists through run polling.
- Reduced-motion mode disables spinner animation.
- Zero browser page errors.
- Screenshots inspected at desktop and 390px: `/tmp/run-live-1440.png`, `/tmp/run-live-390.png`; additional screenshot `/tmp/run-live-375.png`.

The sandbox blocked local listening and Chromium initially; approved elevated commands completed these checks. This is fixture-backed verification, not inspection of the live run or deployment. Preview server stopped after verification. No cluster changes, merge or overall stage transition.

## Constraints

AUTONOMIA-01/02 satisfied: no infrastructure mutation; acceptance demonstrated by commands. SECURITY-05/08/13/15 preserved through existing authenticated requests, escaped React text, safe error messages and handled promises. Other security rules have no new applicable surface in this change. No dependency changes or new serialization/algorithm work. Retry timers clean up and requests stop after readiness or bounded failure.
