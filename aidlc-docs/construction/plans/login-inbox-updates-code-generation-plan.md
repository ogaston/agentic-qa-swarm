# Login persistence and Inbox updates

Approved by the user: “Implement the plan.” Scope: existing U6 dashboard and U1 notification REST projection. Preserve Spanish UI, current confirmation requirements, existing work, and deployment state.

- [x] 1. Persist versioned token/expiration in localStorage after login; validate stored state and server session before protected content. Clear credentials on logout, expiration and 401; handle storage failures. Update session documentation and regression tests.
- [x] 2. Fetch all notifications, derive global stats and filter table rows locally. Keep tabs at the card header's upper right through loading/error/empty states. Refresh globally after existing actions.
- [x] 3. Expose created_at from persisted event occurred_at through the notification REST projection and OpenAPI; regenerate frontend types. Add Spanish relative timestamps with minute updates, exact time and invalid-date fallback.
- [x] 4. Show artifact kind and final seven reference characters with a complete hover/focus tooltip. Keep full modal references. Link repository/commit in both modal steps to GitHub with external-link arrows and keyboard focus trapping.
- [x] 5. Run dashboard tests, lint, build and API type check; ui-api Go tests; verify desktop/mobile layouts at 1440/390/375px. Record evidence and update state.

Interfaces: additive created_at date-time on Notification, session context restoration state if needed; existing API endpoints and bearer authentication retained. Defaults: GitHub repositories, seven trailing artifact characters, notification event creation time, Spanish relative text. No deployment or global stage transition.
