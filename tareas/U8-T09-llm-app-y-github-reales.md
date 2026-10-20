# U8-T09 — Cierre con el humano: LLM real (DeepSeek vía LiteLLM), app definitiva y webhook de GitHub

**Unidad:** U8 — Producto demostrable (ciclo completo en kind)
**Historias que implementa:** US-M1 (evento real de GitHub), US-M3/M4/M9 con modelo real; decisiones C-82 y C-83.
**Depende de:** U8-T08 **fusionada** y tres insumos del humano, **entregados el 2026-10-20** (ver «Insumos del humano»). **Ola 5**. **Tope: 3 rondas**.

---

## Insumos del humano (2026-10-20)

1. **Proveedor LLM:** gateway LiteLLM del humano, compatible con OpenAI, alcanzable desde el host por la red privada del humano. Modelo **`Deepseek-V4-Flash`** (el nombre `deepseek/deepseek-flash` da 403). Es un modelo de razonamiento: con `max_tokens` bajo deja `content` vacío, así que planner y reporter fijan un tope holgado y fallan con error claro si `content` llega vacío.
2. **App definitiva:** la app objetivo que ya existe (U8-T04, `docs/operaciones/app-objetivo.md`); su baseline es el actual.
3. **Webhook de GitHub:** túnel **`cloudflared`** (instalado en el host) a un port-forward del inbox.

**Nada de la red del humano entra al repo.** La clave está en `${XDG_RUNTIME_DIR}/aqs-kind/deepseek.key` y la dirección del gateway en `${XDG_RUNTIME_DIR}/aqs-kind/litellm.env` (modo `600`, variables `LITELLM_UPSTREAM_IP`, `LITELLM_UPSTREAM_PORT`, `LITELLM_UPSTREAM_MODEL`). `scripts/kind/secrets.sh` lee ambos y los materializa en un Secret/ConfigMap creado en el clúster; los manifiestos del repo solo llevan marcadores. El codificador no imprime esos valores ni los escribe en bitácoras, informes o logs.

**Egress (sustituye a «443 del proveedor»):** planner y reporter salen solo a `<LITELLM_UPSTREAM_IP>/32:<LITELLM_UPSTREAM_PORT>` (el gateway del humano, que ya es un LiteLLM), más DNS y, el reporter, MinIO; la NetworkPolicy se renderiza en `scripts/kind/` con esos valores locales.

**Decisión de diseño (ronda 3, humano):** no se despliega un proxy LiteLLM en el clúster. El gateway del humano se usa directamente como endpoint compatible con OpenAI: `LLM_PROVIDER=http` en planner y reporter, con `LLM_BASE_URL` y `LLM_MODEL` leídos de un Secret (`llm-upstream`) y la clave como archivo de Secret (`agents-llm-key`), ambos creados por `scripts/kind/secrets.sh`.

## Alcance

**Dentro** (una línea, concreta):

> Pasar planner y reporter a `LLM_PROVIDER=http` en kind apuntando directamente al gateway del humano (endpoint compatible con OpenAI leído de `litellm.env`; modelo y clave como Secrets creados en el clúster, nunca en el repo; egreso de planner y reporter acotado a ese `IP/32:puerto`, sin proxy en el clúster), apuntar `RUN_ARTIFACT_REF` y el baseline a la app definitiva, y recibir un evento real de GitHub; repetir `kind-e2e.sh` con todo real.

Detalle:

- La clave la entrega el humano fuera del repo; `scripts/kind/secrets.sh` la lee de un archivo local (`${XDG_RUNTIME_DIR}/aqs-kind/deepseek.key`, modo `600`) y crea el Secret `agents-llm-key`. Nunca en la salida ni en logs (comprobado como en `kind-smoke`).
- NetworkPolicy: egress de planner/reporter solo a la IP/puerto de `litellm.env` (además de DNS y, el reporter, MinIO) (C-83).
- Webhook de GitHub: según lo que elija el humano (túnel `cloudflared` a un port-forward del inbox); se documenta el paso exacto.
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

- [ ] **CA-1** — Sin claves en git ni en logs, y planner y reporter disponibles con LLM real.
  ```bash
  git grep -c -i -E 'sk-[a-z0-9]{20,}' | wc -l; bash scripts/kind/kind-smoke.sh 2>&1 | grep -E '^(OK|FALLA) (secrets-no-en-logs|llm-real-configurado)'
  ```
  Esperado: `0`, `OK secrets-no-en-logs` y `OK llm-real-configurado`.

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
  git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(deploy/flux/(base|kind)/|scripts/kind/|docs/operaciones/|demo/|agents/|tareas/U8-T09-llm-app-y-github-reales\.md|bitacoras/U8-T09\.md|revisiones/U8-T09/)' | wc -l
  ```
  Esperado: `0` y `0`. (Ronda 4: `agents/` entra en la lista solo por la excepción http de `check_base_url`, y la propia tarea, autorizado por el humano.)

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
