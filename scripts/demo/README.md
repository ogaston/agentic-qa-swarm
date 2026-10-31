# Local demo

Start with `bash scripts/demo/up.sh` and stop with `bash scripts/demo/down.sh`. The first start creates the kind cluster, builds and loads platform images, deploys the services, starts the dashboard and tunnel, and registers a GitHub push webhook. Later starts reuse the cluster and webhook. Shutdown preserves their state.

Requirements: podman, kind, kubectl, cloudflared, gh, jq, Node dependencies, a configured Cloudflare tunnel named `aqs-demo`, and GitHub CLI authentication with permission to administer webhooks on the target repository. Existing platform secret requirements still apply. `AQS_DEMO_PUBLIC_URL` defaults to `https://aqs.ogaston.com`; The target repository is `ogaston/demo-target-app-qa-swarm`.

Create a demo commit using `bash scripts/demo/10-push-commit.sh --repo-dir /path/to/target --bug off` (or `--bug oversell`). Wait until it reports the image published, then confirm the notification in the dashboard. The credentials are in the protected state file reported by `up.sh`.

To clear the warm database and cache and run a verified reset, use `bash scripts/demo/up.sh --reset`. This requires the cluster and go-reset to be running. The command fails if reset verification fails.
