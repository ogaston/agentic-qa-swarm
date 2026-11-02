# Project onboarding UI refinement

## Context

The completed onboarding slice already has the project selector, wizard, and resource cards. This refinement makes the Inbox selector the single project entry point, removes the redundant Repositories item from primary navigation, and improves the wizard's resource presentation.

## Unit context

- **Stories**: self-service project onboarding and scoped Inbox review.
- **Dependencies**: existing `/api/repositories` list, `/repositories/new` wizard route, editorial design tokens, React Router.
- **Boundaries**: dashboard refinement plus a defensive `ui-api` creation guard. No persisted-data, Kubernetes, or deployment changes.
- **Assets**: use official, unmodified PostgreSQL, Redis, MySQL, and MongoDB logos from their primary brand sources. Store only the necessary SVG assets in `web/dashboard/public/`.

## Execution plan

- [x] **Step 1 — Inspect the existing selector, navigation, wizard, tests, and styles.**
  - Confirm `/inbox` already loads projects and can route an add-project selection to `/repositories/new`.
  - Confirm `/repositories` is only a redundant primary-navigation entry; preserve its direct route.
  - Identify the resource cards and their test coverage.

- [x] **Step 2 — Make Inbox the clear project entry point.**
  - Keep a native, keyboard-accessible project dropdown in Inbox with all projects and a final `Crear nuevo proyecto` action.
  - Route that action to `/repositories/new`, preserve stable test IDs, and clarify label/copy where needed.
  - Remove the Repositories entry from `NavPrincipal` only; do not remove its direct route or wizard links.

- [x] **Step 3 — Refine wizard resource selection.**
  - Group resource cards under visible categories: `Bases de datos` and `Caché y mensajería`.
  - Replace generic resource glyphs with official, unmodified logo assets. Keep status and disabled state in text, not colour alone.
  - Improve responsive spacing, focus treatment, and stepper/card consistency without changing onboarding behavior.

- [x] **Step 4 — Update focused tests.**
  - Cover Inbox add-project navigation and the absence of the primary Repositories link.
  - Cover resource category headings, official-logo alternatives, and supported/disabled controls.

- [x] **Step 5 — Verify frontend quality.**
  - Run focused Vitest coverage, then dashboard lint and production build.
  - Run the existing onboarding browser flow when its fixture setup remains compatible; report any external prerequisite instead of changing backend behavior.

## Completion criteria

- Inbox has one accessible project selector with a create-project entry.
- The main menu no longer lists Repositories.
- Resource choices are grouped by category and display official marks.
- Existing project creation, resource selection, and direct `/repositories` access keep working.
- Relevant tests, lint, and build pass.

## Approved refinement request — 2026-11-02

- [x] **Step 6 — Put project resources in Entorno.**
  - Load registered projects alongside the aggregate warm state and show each project's selected resources in the Entorno view.
  - Keep the global warm status and failures readable if the project list is unavailable.

- [x] **Step 7 — Align Inbox and form controls.**
  - Move the Project selector to the right end of the Inbox notification header on desktop while preserving a responsive mobile layout.
  - Make the project description control use the same geometry and label rhythm as other form inputs.

- [x] **Step 8 — Balance resource categories.**
  - Keep three database cards and add two disabled cache choices so both categories contain three equal-size cards.
  - Do not send unsupported resources to the API.

- [x] **Step 9 — Diagnose and correct project creation.**
  - Trace the complete `POST /api/repositories` path and reproduce the visible failure with a regression test.
  - Surface an actionable, inline error and correct the frontend or configuration boundary responsible for the failure without weakening secret handling.

- [x] **Step 10 — Verify the revised experience.**
  - Run focused dashboard and relevant Go tests, lint, production build, and browser scenario.

## Approved correction request — 2026-11-02

- [x] **Step 11 — Reproduce and identify the invalid-JSON creation failure.**
  - Trace the dashboard request and `ui-api` response path using the reported support reference as evidence, without exposing secrets.
  - Add a regression test for the response shape or serialization boundary that caused `bad_request JSON inválido`.

- [x] **Step 12 — Correct repository creation handling.**
  - Make valid creation responses deserialize reliably and retain an actionable inline error for unavailable credentials or malformed upstream responses.

- [x] **Step 13 — Rebuild the onboarding stepper from the supplied reference.**
  - Use completed, current, and pending semantic states with accessible labels and responsive layout.
  - Preserve the existing wizard flow and controls.

- [x] **Step 14 — Verify the correction and visual regression.**
  - Run focused dashboard and `ui-api` tests, lint, production build, and the existing onboarding browser scenario.

## Approved matching-release correction — 2026-11-02

- [x] **Step 15 — Inspect the active Kind control plane without exposing secrets.**
  - Verify the current Kubernetes context, deployed images, pod image IDs, project-field behavior, and required `ui-api` webhook-key wiring.
  - Confirm the target release is the current workspace source for both `go-governance` and `ui-api`.

- [x] **Step 16 — Verify the matching project-aware release.**
  - Run focused `go-governance` and `ui-api` tests covering project metadata, resources, proxying, and credential wrapping.
  - Build both service images from the same workspace revision.

- [x] **Step 17 — Reconcile and restart both control-plane services together.**
  - Load both images into the guarded `kind-aqs` cluster.
  - Apply the scoped current-manifest fields so `ui-api` receives the webhook-key environment variable and mount without reverting the unrelated `warm-app` image.
  - Restart `go-governance` and `ui-api`, then verify both rollouts and active image IDs.

- [x] **Step 18 — Retry the project-aware contract and record evidence.**
  - Exercise the active control-plane request boundary with `name`, `description`, and `resources` without creating an unintended persistent project.
  - Run focused smoke checks and document the release alignment and results.
