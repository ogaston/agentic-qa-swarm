# Landing CTA background

Scope: the public landing's “Ship without the QA backlog” section. This is a focused visual follow-up to the frontend unit; no business logic, API, data, or infrastructure changes are required.

The existing warm editorial palette and fine geometric lines guide the background. Use a self-hosted SVG with quiet orbital outlines and small violet nodes, leaving space around the heading and access link. Preserve all existing workspace changes.

1. [x] Inspect the section, existing palette, and relevant frontend commands.
2. [x] Add the decorative SVG and section-scoped responsive background styles.
3. [x] Run the dashboard build and existing landing tests; inspect desktop and mobile screenshots.
4. [x] Record the result and update the project state without advancing its overall stage.

Acceptance: `npm run build` and `npm test -- src/pages/Landing/LandingPage.test.tsx` pass in `web/dashboard`; a local browser check confirms the SVG loads, no horizontal overflow at 1440px and 390px, and the CTA link still points to `/login`.

The user requested continuation of the announced plan. No new conditional design stages apply to this decorative change.

## Follow-up: hero preview labels

1. [x] Remove the two floating label containers (100% Reset Parity, WARM-07, and 0 Staging Drift) and ENV badge.
2. [x] Run `npm run typecheck` and the existing landing tests: passed, 3 tests.
3. [x] Clarify which `ed-container` scope the user wants removed, then implement and verify that additional request. The user selected width/padding wrappers everywhere; all seven class usages and their CSS constraints were removed. Decorative corner marks now stay inside the horizontal boundary. Build, 3 existing tests, and browser checks at 1440px and 390px pass.
