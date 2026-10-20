# Ronda 5 — respuesta a `ronda-4.md` (U8-T09)

F-01: Corregido en <sha-del-commit-de-ronda-5>: `docs/operaciones/kind-local.md` (tabla «Planner y reporter en el clúster»,
sección «LLM en kind» reescrita con el estado final: `LLM_PROVIDER=http`, ConfigMaps de fixtures sin montar, y lista de lo que
depende de fixtures falsas: solo `scripts/test/u8-agents-kind.sh` y los pasos `planner-fixture`/`reporter-fixture` del e2e, que
con LLM real se omiten; el resto del guion corre con el modelo real; guion del planner marcado como solo LLM falso; restauración
de fixtures condicionada a que se tocaron). `deploy/flux/kind/kustomization.yaml`: comentarios de la excepción http (ronda 3-5)
y de los ConfigMaps de fixtures.

F-02: Fuera de alcance: tarea candidata P4, propuesta. Persistir `usage` del LLM en los agentes para medir tokens por
corrida de demo. No se implementa la medición. Anotado en `docs/operaciones/kind-local.md` («Costo en tokens (no medido)») y
en la bitácora (ronda 5). Queda a la aceptación explícita del humano al fusionar.

F-03: Corregido en <sha-del-commit-de-ronda-5>: `_is_private_ip_literal` en `agents/agent-planner/src/agent_planner/llm_http.py`
y `agents/agent-reporter/src/agent_reporter/llm_http.py` devuelve False para link-local (169.254/16, fe80::/10), no especificada
(0.0.0.0, ::), multicast y loopback. RFC 1918, ULA fd00::/8 y 100.64.0.0/10 siguen aceptadas (solo con host exacto y bandera).
Solo restringe. Pruebas rojo→verde: `5 failed, 28 passed` antes del cambio y `33 passed` (planner) y `34 passed` (reporter) después.
Caso `LLM_ALLOW_INSECURE_HTTP=1` con host vacío: prueba en ambos `test_llm_http.py` y en `agents/agent-planner/tests/test_config.py`.
Detalle en la bitácora, sección «Ronda 5».

F-04: Corregido en <sha-del-commit-de-ronda-5>: `git update-index --chmod=+x scripts/kind/github-webhook-check.sh` y
`chmod +x`. `git ls-files -s` muestra `100755`.
