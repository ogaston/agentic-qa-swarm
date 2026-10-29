# Live tests in run detail — code generation plan

## Context and requirements

Request: `/runs/run-7e56e11a98a8bce7` must show a loading sign for the current step instead of “actual”, and show tests immediately while running. This is a focused refinement of the existing U6/U10 customer run view. Existing local changes must be preserved. This plan is the source of truth for this change.

Source inspection: `RunPage.tsx` renders “actual” for the current timeline phase. `PlanFlujos` in `Resultados.tsx` uses `useUnaVez`, so an early `plan_not_ready` response never recovers automatically. Flow details are collapsed by default. The plan is already requested from rehearsing onward.

Interpretation: show available planned test flows and their steps during execution. Do not invent per-test execution outcomes that the current FlowPlan API does not provide. Present tests before the unfinished report while the run is active.

Dependencies: existing Run and FlowPlan contracts and dashboard request handling. No new API, data model, dependencies or infrastructure. Conditional design stages are omitted for this bounded UI correction. Overall CONSTRUCTION stage remains unchanged.

## Execution

- [x] Step 1: Inspect run rendering, plan loading, existing tests and styles; establish the two user requirements and the delayed-plan failure case.
- [x] Step 2: In `web/dashboard/src/pages/Run/RunPage.tsx` and `web/dashboard/src/styles/editorial.css`, replace the active phase’s visible “actual” badge with a small loading spinner. Retain `aria-current="step"`, provide an accessible “En curso” label, and support reduced motion. Completed and pending phases retain their status indicators.
- [x] Step 3: Update `web/dashboard/src/pages/Run/Resultados.tsx` to request the plan immediately when eligible and retry transient `plan_not_ready` responses on a bounded interval while the run is active. Stop after success, permission errors, unmount or the retry budget; provide manual retry after exhaustion. Keep report fetching separate. Prefer component-local loading behavior over altering the shared `useUnaVez` contract. In `RunPage.tsx`, place tests before the pending report while active. Expand individual test flows when execution starts, including direct entry into a running run, preserving user toggling afterward and existing terminal behavior.
- [x] Step 4: Extend existing run tests in `RunPage.test.tsx`, `RunPage.resultados.test.tsx` and `RunPage.fidelidad.test.tsx` as appropriate: accessible active indicator without visible “actual”; direct running entry with expanded test steps; rehearsing-to-running transition; delayed plan recovery; bounded retries and cleanup; no repeated requests after success; terminal and access-error behavior.
- [x] Step 5: From `web/dashboard`, run `npm test`, `npm run lint`, and `npm run build`. Verify desktop and mobile presentation with the available local browser tooling. Record evidence and limitations in `aidlc-docs/construction/run-live-tests/code/verification.md`; mark steps complete and update `aidlc-docs/aidlc-state.md`.

## Acceptance and constraints

Commands in Step 5 must pass. Regression tests must demonstrate the delayed-plan recovery and immediate running view without requiring a page reload. Spinner accessibility and flow expansion must be asserted in existing test suites. No infrastructure changes are required (AUTONOMIA-01); acceptance is command-verifiable (AUTONOMIA-02).

Security: retain authenticated API access and generic user-facing errors (SECURITY-08/15); render API content as React text (SECURITY-05/13). Other security rules introduce no new applicable requirements in this UI-only change. Retry behavior must be bounded and cleaned up. No algorithmic or serialization changes require property-based tests.

## Approval

A) Approve this plan and implement the changes.

B) Request changes to the plan.

[Answer]: Approved by user: “go ahead”.
