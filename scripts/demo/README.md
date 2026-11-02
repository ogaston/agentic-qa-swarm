# Local demo

The full up/down guide, with every piece, what breaks it and how to fix it, is
[docs/operaciones/demo-subir-bajar.md](../../docs/operaciones/demo-subir-bajar.md).

| Command | What it does |
|---|---|
| `bash scripts/demo/up.sh` | Starts the kind node, deploys what is missing, rebuilds and recreates the platform images, reconnects the dashboard and the go-intake tunnel backend, and reconciles the GitHub push webhook. |
| `bash scripts/demo/up.sh --reset` | Clears the warm database and cache and runs a verified reset. Fails if verification fails. |
| `bash scripts/demo/status.sh` | Read-only check of every piece; each `FALLA` line prints the command that fixes it. |
| `bash scripts/demo/reconnect.sh` | Restarts the local port-forwards (dashboard and go-intake) after pods were recreated, without rebuilding. |
| `bash scripts/demo/down.sh` | Stops the dashboard, the port-forwards and the kind node. Cluster data and the webhook are preserved. |
| `bash scripts/demo/10-push-commit.sh --repo-dir <target> --bug off\|oversell` | Pushes a demo commit to the target app; wait for its image, then confirm the notification in the dashboard. |

Requirements: podman, kind, kubectl, gh (with permission to administer webhooks on `ogaston/demo-target-app-qa-swarm`), jq, curl, python3, Node dependencies, and the `cloudflared` systemd service publishing `AQS_DEMO_PUBLIC_URL` (default `https://aqs.ogaston.com`) to `127.0.0.1:18622`. The webhook secret is the platform secret, or the per-repository derived secret when the repository is registered in the dashboard's project wizard; `up.sh` reads the registry and sets the right one.
