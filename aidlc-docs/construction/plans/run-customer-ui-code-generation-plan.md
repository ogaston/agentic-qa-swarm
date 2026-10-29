# Customer run view — code generation plan

## Context and scope

Existing U6/U10 run-detail interface, requested for customer usage. Source inspection confirms platform trace IDs, controller/reporter names, platform API routes, workflow identifiers, and implementation flags in visible content. Preserve current local edits, including the run creation retry fix. This plan is the source of truth for this change.

Traceability: customer request for /runs/run-9b5bd46c5dd16719; existing run results and flow inspection functionality. Dependencies: existing Run, Report and FlowPlan contracts. No new data model or infrastructure changes. Conditional design stages are omitted because this is a presentation change.

## Execution

- [x] Step 1: Inspect run view and result components; identify customer-facing content and internal details.
- [x] Step 2: Update `web/dashboard/src/pages/Run/RunPage.tsx` with a clear run heading, human-readable status and progress, customer language for phases, and simple automatic-update messaging. Remove traces, internal state codes and component names. Preserve polling/retry behavior.
- [x] Step 3: Update `web/dashboard/src/pages/Run/Resultados.tsx`: use “Resultados de la prueba”, friendly verdicts and flow labels; remove platform API routes, reporter/workflow identifiers and raw implementation flags. Keep the customer's tested methods, routes, request examples and business rules. Inspect dynamic report/evidence content for internal infrastructure references and show customer-safe content without inventing results or breaking evidence access.
- [x] Step 4: Apply small scoped visual improvements in `web/dashboard/src/styles/editorial.css` (and existing styles only if necessary): clearer heading hierarchy, balanced spacing, tidy report and flow cards, and readable responsive timeline. Preserve the current editorial design and accessible interactions.
- [x] Step 5: Update affected existing tests for customer-facing copy and verify removal of internal details. Run `npm test`, `npm run lint`, and `npm run build` from `web/dashboard`; inspect the local view at desktop and 375/390px, including expanded flows and errors. Record verification in `aidlc-docs/construction/run-customer-ui/code/verification.md` and update state.

## Approval

Approve this plan to implement it, or request changes.

[Answer]: Approved by user: “approevd”.

## Requested refinement — inner table

User requested a simpler, lighter inner flow table after reviewing the implementation.

- [x] Step 6: Remove the inner table frame and header fill; use sentence-case headings, light row dividers and plain inline code. Retain flow container, semantic columns and responsive stacked fields. Verify build and browser at desktop/mobile widths.
