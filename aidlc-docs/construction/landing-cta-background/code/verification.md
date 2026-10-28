# Landing CTA background verification

The final public CTA now uses a self-hosted orbital SVG on the existing linen background. Fine stone outlines and violet nodes frame the heading without changing its copy, navigation, or button behavior. Responsive positioning keeps the mobile text clear. The asset contains no scripts, external references, fonts, or animation.

Changed application files:

- `web/dashboard/src/styles/editorial.css`: scoped background and mobile positioning for `.ed-cta`.
- `web/dashboard/public/cta-orbits.svg`: decorative vector background.

Verification on 2026-10-28:

- `npm run build`: passed, including TypeScript.
- `npm test -- src/pages/Landing/LandingPage.test.tsx`: 3 tests passed.
- `node /tmp/check-landing-cta.mjs`: passed against the built landing at 1440px and 390px. The SVG returned HTTP 200, the content was visible, no horizontal overflow or browser page errors occurred, and the CTA retained `/login`.
- Visually inspected `/tmp/landing-cta-1440.png` and `/tmp/landing-cta-390.png`.

The initial dev-server attempt lacked required proxy configuration; verification instead served the production build locally. The sandbox blocked local sockets and Chromium; the visual check succeeded after the tool's escalation approval. Graph symbol lookup located `LandingPage`; coverage lookup was unavailable under the tool approval policy, so exact source files were inspected directly.

No infrastructure changes or advance to the project's overall Build and Test stage. Existing unrelated workspace changes were preserved.

## Follow-up: preview labels and full-width layout

Removed the two floating hero label containers and environment badge requested by the user. After the user selected “Remove width/padding wrappers everywhere,” removed all seven `ed-container` class usages in `LandingPage.tsx` and their base and responsive CSS constraints. Contents and reveal classes remain intact. Decorative corner marks now sit inside the preview's horizontal boundary because their previous 5px exterior offsets caused mobile overflow after removing container padding.

Validation: the dashboard build (including TypeScript), 3 existing landing tests, and `git diff --check` pass. Local browser checks at 1440px and 390px pass with no horizontal overflow or page errors; the background asset returns HTTP 200 and the CTA retains `/login`. The final mobile screenshot was visually inspected.
