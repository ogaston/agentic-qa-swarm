# Inbox project selector and project-onboarding wizard

## Context

The inbox currently shows every notification without project filtering. `/repositories` already registers a repository and triggers a warm namespace, but its creation modal has no project metadata, resource choices, credential delivery, or live validation. Provisioning always creates PostgreSQL and Redis, and reset assumes both exist.

This change will deliver one vertical slice: a GitHub-labelled project selector in the inbox, a protected `/repositories/new` wizard, resource-aware Kubernetes provisioning and reset, per-project webhook credentials, and a waiting screen that completes only after infrastructure provisioning and a real signed GitHub webhook are observed.

## Approach

Reuse the existing repository registry, warm-manager reconciler, UI API, event stream, editorial styles, native controls, and inline SVG convention. Do not add a GitHub App, collect a GitHub token, or open cluster egress to GitHub. The user configures the webhook manually; a signed `ping` or normal accepted event validates the configuration.

Only PostgreSQL 16.4 and Redis 7.4.1 will be enabled. Other resource cards will be visible but disabled with `Próximamente`. Resource selections will be enforced by the backend, not only represented in the UI.

## Key changes

- [x] **Step 1 — Extend contracts and persisted project metadata.**
  - Update `contracts/openapi/control-plane.yaml`: add project `name`, optional `description`, `resources` (`postgresql`, `redis`), and `webhook_secret_version` to repository models; accept name/description/resources in `RepositoryInput`; add creation and credential-rotation response schemas; add `infrastructure_ready` to warm status.
  - Add an owner/admin `POST /repositories/{owner}/{repo}/webhook-secret/rotate` operation. Keep repository creation available to any authenticated user, as today.
  - Add `contracts/events/webhook.validated.schema.json` plus valid/invalid examples for signed GitHub `ping` activity. Do not turn pings into inbox notifications.
  - Update `services/go-governance/internal/repos/` and repository service/server tests. Default omitted resources to both supported resources and omitted names to `repo`, preserving existing clients and stored records. Version `0` means a legacy project using the platform webhook secret; new projects start at version `1`.
  - Regenerate `web/dashboard/src/api/schema.gen.ts` and add golden validation for resource names, empty resource lists, and credential versions.

- [x] **Step 2 — Make warm provisioning resource-aware and report infrastructure readiness separately.**
  - Extend `services/go-warm-manager/warmtarget/warmtarget.go` and `cmd/go-warm-manager/registry.go` to carry validated resource selections.
  - Update `provision/template.go` and `provision/reconcile.go` to create only selected PostgreSQL/Redis workloads, services, and database credentials. Keep namespace, app placeholder, baseline token, quotas, limits, RBAC, and NetworkPolicies for every project. Omit `DATABASE_URL` when PostgreSQL is not selected.
  - Store the selection and infrastructure status in `warm-policy`. Report `infrastructure_ready=true` only after all required Kubernetes objects exist and selected database/cache workloads are ready; do not require the placeholder app to pass QA readiness and do not set `reset_verified` prematurely.
  - Preserve the legacy `aqs-test` template and default-to-both behavior. Add parity, reconciliation, idempotency, PostgreSQL-only, Redis-only, and stateless tests.

- [x] **Step 3 — Make verified reset honor selected resources.**
  - Read the resource selection from `warm-policy` in `services/go-reset` when constructing a dynamic target.
  - Skip database cleaning/version checks when PostgreSQL is absent and Redis flush/size checks when Redis is absent. Always retain app restart/readiness, state transitions, events, quarantine, and read-back verification for selected resources.
  - Use explicit no-op/stateless adapters rather than nil interface calls. Add reset tests for all four selection combinations and prove legacy targets still run the full PostgreSQL/Redis reset.

- [x] **Step 4 — Add isolated per-project webhook secrets and signed-ping activity.**
  - Derive project secrets as `whsec_` + hex HMAC-SHA256 of the platform webhook key, a versioned context string, normalized `owner/repo`, and `webhook_secret_version`. Add shared golden vectors so `go-intake` and `ui-api` cannot diverge. Raw project secrets are never persisted.
  - Extend intake lookup metadata to distinguish legacy/global-secret projects from managed/versioned projects. Extract only `repository.full_name` to choose the key, then verify HMAC before normal processing. Preserve global-secret behavior for legacy repositories and fail closed for managed projects.
  - After a valid, allowed `ping`, publish `webhook.validated`; a publish failure returns a retryable error. Keep ping out of notifications and `notify.created`.
  - Extend `services/ui-api/inbox` file and NATS projections to merge validated pings with existing notification activity. `/repositories/{owner}/{repo}/activity` then reports `has_webhook`, latest event/time, and optional SHA after either a ping or an accepted normal event.
  - Test wrong keys, legacy compatibility, disabled/unregistered projects, key rotation, replay/idempotency, and the absence of secrets in logs, audit entries, ordinary repository responses, and event payloads.

- [x] **Step 5 — Deliver and rotate credentials through UI API without storing them.**
  - Mount the existing `go-intake-webhook` key read-only in `ui-api` and read it through a file-based setting, consistent with other service credentials.
  - On successful repository creation, return the repository plus the derived webhook secret once. Return no secret from list/detail/activity endpoints. Keep the existing `Cache-Control: no-store` behavior.
  - Implement rotation by incrementing the governance credential version as owner/admin, then returning the new derived secret once. This provides recovery if the user leaves the wizard before copying the secret or if a secret is compromised.
  - Continue using the configured public intake URL for copy-and-paste instructions. Never mount or return internal `warm-db-credentials` or `target-app-baseline-token` values to the browser.
  - Add API authorization, response-shape, no-storage/no-log, rotation, and leak tests; update deployment wiring and relevant security tests without widening Kubernetes RBAC.

- [x] **Step 6 — Add the GitHub project selector to the inbox.**
  - In `web/dashboard/src/pages/Inbox/InboxPage.tsx`, load `/api/repositories` alongside notifications and add a native `<select>` with the existing inline-SVG style and an official GitHub mark.
  - Include `Todos los proyectos`, each registered project, and `Añadir nuevo proyecto`. Selecting the add option navigates to `/repositories/new`; selecting a project filters rows, tab counts, KPI values, pagination, and warm status to that project. `Todos los proyectos` preserves current global behavior.
  - Support `/inbox?project=owner%2Frepo` so wizard completion can return to the selected project. Reset pagination and close any open notification when changing projects. Keep loading/error states usable and preserve notifications from legacy/unregistered projects under the global view.
  - Add stable `data-testid` attributes, visible labels, keyboard coverage, and tests for filtering, counts, repository-loading failure, and add-project navigation.

- [x] **Step 7 — Build the three-step wizard and consolidate existing onboarding.**
  - Add `web/dashboard/src/pages/Repositories/ProjectWizardPage.tsx` and protect `/repositories/new` in `App.tsx`. Reuse editorial tokens/styles and the useful visual ideas in `mockups/onboarding-mock.html`, not its unsupported five-step feature set.
  - Step 1 collects required project name and optional description, plus the existing App Contract confirmations. The database confirmation appears only when PostgreSQL is selected; baseline port and GHCR publication remain required.
  - Step 2 uses native checkbox cards with inline SVG resource icons. Enable PostgreSQL and Redis, default both selected, and allow either or neither for stateless apps. Show MySQL/MongoDB and other illustrative resources disabled with text, not color alone.
  - Step 3 collects normalized `owner/repo` and GHCR image prefix, with client and server validation. Submit exactly once, then show the payload URL and per-project webhook secret with copy buttons and precise GitHub instructions for JSON, push, pull-request, and release events.
  - While the user copies/configures GitHub, poll repository detail and activity with bounded backoff, a visible elapsed/waiting state, manual retry, and separate infrastructure/webhook statuses. Finish only when `infrastructure_ready` and `has_webhook` are both true; then link to the filtered inbox and repository list. Never persist the secret in localStorage, sessionStorage, URLs, logs, or analytics.
  - Use semantic forms, labels, fieldsets, blur validation, focused error summaries, `aria-current="step"`, live waiting announcements, 48px touch targets, responsive layout, and reduced-motion behavior from the retrieved `forms` guidance.
  - Point the existing `/repositories` add action to the same wizard, remove the superseded creation modal, update its webhook copy, and show project name/resources in the list. Keep existing enable/delete/activity behavior.

- [x] **Step 8 — Verify the complete user scenario and record AI-DLC evidence.**
  - Copy the approved plan to `aidlc-docs/construction/plans/project-onboarding-code-generation-plan.md`, mark steps as they complete, append all required audit entries, and update `aidlc-docs/aidlc-state.md` without changing the global construction stage.
  - Add/extend unit and contract tests beside the changed code. Extend the existing kind self-service scenario to verify selected resource creation/omission, infrastructure readiness, signed ping validation, legacy webhook compatibility, rotation, and no secret exposure.
  - Exercise the production-built dashboard with fixture-backed browser flows at 1440px, 390px, and 375px in light/dark themes: inbox filtering, add-project navigation, all wizard steps, disabled resources, validation errors, copy actions, waiting-to-success, timeout/retry, keyboard navigation, and reduced motion. Confirm no horizontal overflow or browser errors.
  - Write verification evidence to `aidlc-docs/construction/project-onboarding/code/verification.md`. Preserve unrelated existing changes in `aidlc-docs/audit.md` and `scripts/kind/kind-smoke.sh`; do not deploy to dev/prod or create commits unless requested.

## Verification

Run the project’s standard checks on the final deliverable:

```bash
bash contracts/validate.sh
(cd services/go-governance && go test ./...)
(cd services/go-warm-manager && go test ./...)
(cd services/go-reset && go test ./...)
(cd services/go-intake && go test ./...)
(cd services/ui-api && go test ./...)
(cd web/dashboard && npm test -- --run && npm run lint && npm run check:api && npm run build)
bash scripts/ci/rbac-matrix.sh
bash scripts/test/u12-self-service-kind.sh
bash scripts/kind/kind-smoke.sh
```

The kind checks run only against the local `kind-aqs` environment. If kind is unavailable, complete all other checks and report the cluster verification as pending rather than claiming it passed.

## Assumptions

- “Project” extends the existing repository registration; it is not a new multi-tenant hierarchy.
- Resource selection means the supported resources chosen in step 2. PostgreSQL and Redis are independently optional; selecting neither creates a stateless app environment.
- GitHub validation means an actual HMAC-valid webhook received by intake, not merely checking repository syntax or calling GitHub with a PAT.
- Webhook setup remains manual. Automatic GitHub App/OAuth installation and webhook creation are out of scope.
- The only user-facing secret is the per-project webhook secret. Generated database passwords and baseline tokens remain internal Kubernetes Secrets.
- Existing repositories keep the shared legacy webhook secret until explicitly migrated; new projects use isolated, rotatable derived secrets.
- UI copy remains Spanish, consistent with the current dashboard, and no new frontend dependency is introduced.
