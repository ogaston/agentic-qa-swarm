# Project onboarding UI refinement follow-up verification

## Delivered

- Entorno lists the selected components for every registered project separately from the aggregate warm state.
- Inbox places the Project selector at the right side of desktop notification controls and preserves its responsive layout.
- The project description textarea uses the same field geometry as the other form controls.
- Resource choices present three database cards and three cache cards. Memcached and Valkey are disabled and are never sent to the API.
- Repository creation validates that the response contains the one-time webhook credentials and displays an actionable compatibility error otherwise.
- `ui-api` rejects repository creation before proxying it when its webhook derivation key is unavailable, preventing a partially created project.

## Runtime diagnosis

A read-only inspection of the active Kind `aqs-system/ui-api` Deployment found image `ghcr.io/ogaston/agentic-qa-swarm/ui-api:0.0.0` without `UIAPI_WEBHOOK_KEY_FILE` or the webhook-key volume mount. The source Flux manifest includes both. This stale runtime configuration can cause the live add-project failure. No deployment, reconcile, or other cluster mutation was performed.

## Verification

- `npm test -- --run src/pages/Repositories/ProjectWizardPage.test.tsx src/pages/Warm/WarmPage.test.tsx src/pages/Inbox/InboxPage.test.tsx`: 72 passing tests.
- `npm run lint` and `npm run build`: pass.
- `go test ./internal/httpapi/...`: 112 passing tests.
- `git diff --check`: pass.
- `npm run e2e:onboarding`: pass at 1440px, 390px, and 375px in light and dark themes.

## Correction and stepper follow-up

- The reported `bad_request` with `JSON inválido` originates in the governance decoder when a running control plane does not recognize the newer project fields. The wizard now identifies that contract mismatch without suppressing the support reference.
- The active Kind deployments report static `go-governance:0.0.0` and `ui-api:0.0.0` image tags. A matching release must be deployed to make the live creation path accept the project payload; no cluster mutation was performed.
- The four-state stepper uses semantic completed, current, and pending states, visible labels, and a responsive horizontal overflow strategy based on the supplied reference.
- Verification: 7 focused wizard tests, 112 `ui-api` HTTP tests, dashboard lint, production build, `git diff --check`, and browser onboarding at 1440px, 390px, and 375px in light and dark themes all pass.

## Matching-release runtime correction

- Rebuilt `go-governance:0.0.0` and `ui-api:0.0.0` together from the current workspace source, then loaded both images into the guarded `kind-aqs` cluster.
- Applied only the current `ui-api` webhook-key environment variable, volume, and mount. The full overlay was not applied because its diff would also revert the unrelated `aqs-test/warm-app` image.
- Restarted both deployments and verified one available, updated replica for each. The replacement pods started at `2026-11-02T14:31:15Z` and `2026-11-02T14:31:16Z` with new image IDs.
- Retried the active authenticated repository boundary with `name`, `description`, and `resources`, using an intentionally invalid repository identifier. The response was `422 invalid_input`, proving the project fields passed strict JSON decoding. Repository count remained `0` before and after.
- `go test ./...` passed with 366 tests in `go-governance` and 349 tests in `ui-api`.
- All seven control-plane `/healthz` and `/readyz` checks passed, as did login, Inbox auth, RBAC, NetworkPolicy, log-secret hygiene, and LLM configuration checks.
- The wider preflight remained non-green only because the pre-existing warm target is in `cuarentena` with `reset_verified=false`. No reset was performed because it is outside this control-plane release correction.
- `git diff --check` passes.
