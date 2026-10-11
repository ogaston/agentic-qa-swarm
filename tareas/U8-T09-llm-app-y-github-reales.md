# U8-T09 — Cierre con el humano: LLM real (DeepSeek vía LiteLLM), app definitiva y webhook de GitHub

**Unidad:** U8 — Producto demostrable (ciclo completo en kind)
**Historias que implementa:** US-M1 (evento real de GitHub), US-M3/M4/M9 con modelo real; decisiones C-82 y C-83.
**Depende de:** U8-T08 **fusionada** y tres insumos del humano: (1) la clave de DeepSeek, (2) la app definitiva (repo o imagen) con su contrato de baseline, (3) cómo llega GitHub al clúster (túnel o reenvío). **No despachable hasta tener los tres.** **Ola 5**. **Tope: 3 rondas**.

---

## Alcance

**Dentro** (una línea, concreta):

> Desplegar LiteLLM en `aqs-system` (imagen fijada, config con `deepseek/deepseek-chat`, clave solo como archivo de Secret creado en el clúster y nunca en el repo, único componente con egress al proveedor), pasar planner y reporter a `LLM_PROVIDER=http` en kind, apuntar `RUN_ARTIFACT_REF` y el baseline a la app definitiva, y recibir un evento real de GitHub; repetir `kind-e2e.sh` con todo real.

Detalle:

- La clave la entrega el humano fuera del repo; `scripts/kind/secrets.sh` la lee de un archivo local (`${XDG_RUNTIME_DIR}/aqs-kind/deepseek.key`, modo `600`) y crea el Secret. Nunca en la salida ni en logs (comprobado como en `kind-smoke`).
- NetworkPolicy: egress de LiteLLM solo a 443 del proveedor; planner/reporter solo a LiteLLM y MinIO (C-83).
- Webhook de GitHub: según lo que elija el humano (túnel tipo `smee`/`cloudflared` a un port-forward, o reenvío firmado desde un script); se documenta el paso exacto.
- Se mide y se anota el costo de tokens de una corrida de demo (P4 queda candidata).

**Fuera**:

- Clúster dev/prod, dominio público, ingress con TLS (candidatas).

---

## Archivos de contexto

- `tareas/candidatas.md` (C-82, C-83), `tareas/U8-T05-agentes-en-cluster.md`, `tareas/U8-T08-ciclo-completo-kind.md`
- `docs/operaciones/app-objetivo.md` (U8-T04), `docs/operaciones/kind-local.md`
- `agents/*/src/*/llm_http.py`

---

## Criterios de aceptación

- [ ] **CA-1** — LiteLLM disponible y la clave fuera del repo y de los logs.
  ```bash
  kubectl -n aqs-system get deploy litellm -o jsonpath='{.status.availableReplicas}'; echo
  git grep -c -i -E 'sk-[a-z0-9]{20,}' | wc -l; bash scripts/kind/kind-smoke.sh 2>&1 | grep -E '^(OK|FALLA) secrets-no-en-logs'
  ```
  Esperado: `1`, `0` y `OK secrets-no-en-logs`.

- [ ] **CA-2** — Ciclo completo con modelo real y app definitiva.
  ```bash
  bash scripts/kind/kind-e2e.sh --bug off 2>&1 | grep -c '^FALLA'; echo "rc=${PIPESTATUS[0]}"
  ```
  Esperado: `0` y `rc=0`.

- [ ] **CA-3** — Evento real de GitHub llega al inbox.
  ```bash
  bash scripts/kind/github-webhook-check.sh; echo "rc=$?"
  ```
  Esperado: `OK webhook-github-recibido` (la entrega aparece en el inbox con el SHA del commit empujado por el humano) y `rc=0`.

- [ ] **CA-4** — Alcance.
  ```bash
  git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(deploy/flux/(base|kind)/|scripts/kind/|docs/operaciones/|demo/|bitacoras/U8-T09\.md|revisiones/U8-T09/)' | wc -l
  ```
  Esperado: `0` y `0`.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
