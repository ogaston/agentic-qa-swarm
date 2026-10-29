# Run creation race — code generation plan

## Context and diagnosis

Target: existing U6 run detail view, stories US-M2/US-M5/US-M6/US-M7.1 and S5. Dependencies: human confirmation in ui-api (U1) and asynchronous event consumption in go-run-controller (U2). No schema, infrastructure, or new business rules are required; conditional design stages are omitted.

`services/ui-api/internal/httpapi/api.go` persists the confirmation receipt, publishes `run.confirmed`, and returns HTTP 201. This does not wait for the controller to consume the event. `services/go-run-controller/runctl/controller.go` creates the run when applying that event. The dashboard navigates immediately after confirmation. `web/dashboard/src/pages/Run/RunPage.tsx` currently treats every HTTP 404 as fatal, stopping polling permanently. This reproduces the mechanism behind the reported screenshot; the particular live run has not been inspected.

Source files were read directly because graph discovery and coverage tools rejected access. The existing 13 RunPage tests pass, including a test requiring immediate permanent termination on 404. That assertion encodes the behavior that needs revision. The historical U6-T05 requirement for a single request on 404 is superseded for the initial lookup by this proposed correction.

## Scope and steps

This plan is the source of truth for this correction. Application changes remain in the workspace root; documentation remains under aidlc-docs. Preserve all existing local edits.

- [x] Step 1: Inspect confirmation, run creation, proxy routing, polling hook, and existing tests; identify the asynchronous visibility window.
- [x] Step 2: Add regression tests in `web/dashboard/src/pages/Run/RunPage.test.tsx`: initial 404 followed by confirmed/running recovers; repeated initial 404s stop after five consecutive failures with a retry button; retry resumes successfully; a 404 after a successful lookup remains fatal; route changes and unmount cancel pending retries. Run the focused suite to demonstrate the regression before implementation.
- [x] Step 3: Modify `web/dashboard/src/pages/Run/RunPage.tsx` to tolerate initial 404s within the existing polling failure budget (five consecutive failures, three-second interval). Show a waiting status while retrying, without the permanent not-found alert. On exhaustion show the not-found alert and existing retry action; do not leave the header at “Cargando…”. After the first successful lookup, preserve fatal-404 behavior. Keep the generic polling hook and backend unchanged.
- [x] Step 4: Verify focused tests, full dashboard suite, lint, API type synchronization and production build: `cd web/dashboard && npx vitest run src/pages/Run/RunPage.test.tsx`, `npm test`, `npm run lint`, `npm run check:api`, `npm run build`. Inspect the diff for unrelated modifications. Record results in `aidlc-docs/construction/run-creation-race/code/verification.md` and update this plan and state.

## Acceptance

The regression commands must prove recovery without reloading, bounded retries for permanently missing runs, retry recovery, unchanged terminal-state behavior, and no polling after unmount. Existing dashboard tests, lint, schema check, and build must pass. No deployment is included.

## Approval

Approval is required by `.aidlc-rule-details/construction/code-generation.md`, Part 1, Step 7: “Do not proceed until the user explicitly approves the unit code generation plan”. The user approved the plan before implementation; implementation and local verification are now complete.

Approve this correction plan?

A) Approve the plan and implement the correction.

B) Request changes to the plan.

C) Other (describe after the answer).

[Answer]: A — approved by the user: “approved, go ahead” (2026-10-29T00:39:14Z).

## Regression baseline

Before implementation, the expanded RunPage suite reports 5 failures and 12 passes. Failures demonstrate permanent termination on the first initial 404, missing retry recovery, and an incorrect loading header. Raw output: `/tmp/aqs-run-race-red.log`.

## Completion

All implementation and verification steps are complete. Focused suite: 17 passed. Full dashboard suite: 241 passed across 21 files. Lint, API schema check, TypeScript and production build pass. Evidence: `aidlc-docs/construction/run-creation-race/code/verification.md`. Awaiting code review; no deployment or global stage transition.
