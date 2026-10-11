# Ronda 1 — U8-T05

VEREDICTO: VERDE

No queda ningún ROJO ni NARANJA. Corrí yo mismo los cuatro criterios de aceptación y todos pasan. No tengo permiso de escritura, así que el orquestador debe guardar este texto en `revisiones/U8-T05/ronda-1.md`.

Revisé sha 30ce407 en `/home/omarjayg/Javeriana/topicos-especiales/agentic-qa-swarm-wt-U8-T05`, con base origin/main 5a098ec. El hash de la tarea no lo recalculé. Al terminar, `git status --short` está vacío: el worktree está limpio.

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Pruebas de los agentes | `cd agents/$a && .venv/bin/python -m pytest -q` en planner y reporter | **pasa**. Planner: 179 passed, 7 deselected, 34 s, rc=0. Reporter: 289 passed, 3 deselected, 32 s, rc=0. Los tiempos son reales, no hay señal de pruebas saltadas. |
| 1b | Prueba con marca `minio` contra MinIO real | Levanté MinIO con la imagen fijada por digest, con podman, y corrí `AQS_S3_TEST_*=… pytest -m minio tests/test_s3_adapters.py` | pasa (1 passed). Ver F-02. |
| 2 | Usuario MinIO con permisos mínimos, leído de vuelta | `bash scripts/test/u8-minio-user-kind.sh` en `kind-aqs` | **pasa**. Salió `OK put-evidence`, `OK get-evidence`, `OK sin-admin`, `OK otro-bucket-denegado`, rc=0, dos corridas seguidas. |
| 2r | Rojo propio | Con `mc admin user remove` quité al usuario `aqs-evidence` y repetí el script | falla como debe: `Access Key Id you provided does not exist` en los cuatro, rc=1. `sin-admin` y `otro-bucket-denegado` salen FALLA porque exigen denegación explícita. |
| 3 | Agentes disponibles y aislados | `kubectl -n aqs-system get deploy agent-planner agent-reporter -o jsonpath=…` y `bash scripts/test/u8-agents-kind.sh` | **pasa**. `agent-planner=1` y `agent-reporter=1`. Salió `OK planner-plan-valido`, `OK reporter-lee-minio`, `OK otro-pod-bloqueado`, rc=0, dos corridas seguidas. |
| 4 | Políticas y alcance | `bash scripts/ci/policies.sh \| grep -c '^FALLA'`; `git status --short \| wc -l`; `git diff --name-only $b \| grep -v -E '…' \| wc -l` | **pasa**. Salió `0`, `0` y `0`. |
| — | Smoke de la plataforma | `bash scripts/kind/kind-smoke.sh` | rc=0, "smoke: OK (sin FALLA)". |

Cómo corrí lo del clúster:
- Todo fue un único bloque `flock ${XDG_RUNTIME_DIR:-/tmp}/aqs-kind.lock`: `kind-up`, `build-images`, `load-images`, `deploy`, los criterios, `kind-down`.
- `kind get clusters` termina mostrando solo `aqs-poc` y `ckad`, así que `aqs` quedó borrado. No toqué `aqs-poc` ni `ckad`.
- Los logs están en `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/3e524589-8d03-4b3f-afd8-e3852d59d1ee/scratchpad/rev/`: `main.log` y un `.log` por paso.

Lectura de vuelta en el estado real:
- **CA-2:** el script escribe y luego lee de MinIO con las credenciales de `aqs-evidence-s3`. No me quedo con el código de salida.
- **CA-3:** `report_uri` se lee desde MinIO y se compara con la respuesta del reporter.
- **Despliegue:** `minio-init` quedó Completed. El log del initContainer dice `usuario de evidencia listo: politica aqs-evidence sobre bucket evidence`.
- **Estado final del clúster:** todos los pods Running, con 0 reinicios en `agent-reporter`. Las NetworkPolicy `agents-ingress-from-controller`, `agent-planner-egress` y `agent-reporter-egress` existen. `allow-same-namespace` excluye `agent-planner`, `agent-reporter` y `nats`.

Lo que no corrí: `shellcheck` y `ruff` no están instalados en el entorno, así que no hubo lint.

## Arbitrajes del orquestador: bien implementados
- **`mc` de la imagen MinIO fijada por digest:** el initContainer `provision-user` en `deploy/flux/base/minio/job-init.yaml` usa `cgr.dev/chainguard/minio@sha256:9dcc028b…`, el mismo digest que el StatefulSet. Funciona en kind con TLS verificado (`SSL_CERT_FILE`), sin `--insecure`. La raíz se compone en `MC_HOST_aqs` desde `minio-root` dentro del proceso y no se imprime. La política es la pedida: `ListBucket` sobre `evidence` y `Get/Put` sobre `evidence/*`, sin admin.
- **Deployments en `agents.yaml`:** `control-plane.yaml` no aparece en el diff. Cada agente tiene Deployment, Service `:8080`, sondas, `securityContext` endurecido (`readOnlyRootFilesystem`, drop ALL, `runAsNonRoot`, seccomp), SA sin token y NetworkPolicy. El egreso es DNS y LiteLLM; el reporter suma MinIO.
- **Secret del LLM:** `agents-llm-key` con clave `api-key`, `optional: true`. `secrets.sh` genera un relleno solo para kind.

## Cambio de `server.py` (fixtures con `prompt` precalculado)
Lo juzgo dentro del alcance. No lo marco como desborde ni como ROJO.
- **Es necesario:** la tarea exige fixtures del LLM falso en un ConfigMap de kind junto con `REPORTER_EVIDENCE=s3`. La forma antigua (`evidence_uris`) leía MinIO al arrancar, así que el pod no arrancaba sin evidencia sembrada. La bitácora lo reproduce con `KeyError: 'evidence_uris'` en rojo.
- **El archivo está permitido:** `agents/agent-reporter/` está en la lista de CA-4 y en el alcance dentro.
- **Está acotado:** solo vive en la rama `fake`, que exige `REPORTER_ALLOW_FAKE=1` y prohíbe `REPORTER_ENV=prod`. Es compatible hacia atrás y trae dos pruebas (`tests/test_fixtures_prompt.py`).
- **Fixtures al día:** regeneré `deploy/flux/kind/agents/reporter/fixtures.json` con `scripts/kind/gen_agent_fixtures.py` y no hay diferencia (`fixtures-al-dia`).
- **Sin salida de la lista permitida:** la lista de archivos del diff cumple CA-4.
- Pido que el orquestador lo registre como decisión explícita. No lo traté como hallazgo.

## Hallazgos
### F-01 · AMARILLO · `deploy/flux/kind/agents/reporter/fixtures.json` · fixture atado a bytes del prompt y de la evidencia
Hoy está al día, pero si cambia la plantilla del prompt de `reporter.py` o el dataset `bug-ecom-01`, el fixture queda obsoleto. El síntoma sería un "prompt no registrado" que solo se ve en kind. No hay chequeo en CI. El codificador ya lo anotó como candidata.

### F-02 · AMARILLO · `agents/agent-reporter/tests/test_s3_adapters.py` · cobertura `minio` parcial y deseleccionada por defecto
- La prueba con marca `minio` solo ejercita `S3ReportStore` contra MinIO real. No ejercita `S3EvidenceReader` ni TLS. Eso queda cubierto después en kind, por `reporter-lee-minio`.
- El `pytest -q` de CA-1 la deselecciona (`-m 'not … minio'`), así que CA-1 por sí solo no prueba contra MinIO. La bitácora tampoco registra haberla corrido con `-m minio`. Yo la corrí y pasa.

### F-03 · AMARILLO · varios archivos · detalles menores
- `s3.py`, `S3EvidenceReader.get`: `except Exception as e` deja `e` sin usar. `ruff` lo marcaría como F841.
- `u8-agents-kind.sh` siembra `runs/run-bug-ecom-01/...` y el reporte en MinIO y no los borra. Es idempotente, porque la clave del reporte lleva el hash del contenido.
- La bitácora no pega la salida de CA-4 (`git status` y el diff de alcance "tras el commit"). Yo los verifiqué: `0`, `0` y `0`.
- No hay prueba de que `REPORTER_EVIDENCE` con un valor desconocido aborta con `SystemExit`.

## Tareas candidatas (defectos reales fuera de alcance)
- Chequeo en CI de que `fixtures.json` se regenera sin diferencia (ya lo había señalado el codificador).
- Publicar `report.ready` por NATS (ya estaba como candidata).
- Prueba de que el egreso de los agentes está restringido, por ejemplo que el planner no alcanza a MinIO. CA-3 solo prueba el ingreso (`otro-pod-bloqueado`).

## Rutas de transcripciones largas
- `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/3e524589-8d03-4b3f-afd8-e3852d59d1ee/scratchpad/rev/` (`main.log`, `06-ca2.log`, `07-ca3.log`, `08-ca2-bis.log`, `09-ca3-bis.log`, `10-smoke.log`, `11-estado.log`, `13-rojo-ca2.log`)

VEREDICTO: VERDE
AMARILLO|deploy/flux/kind/agents/reporter/fixtures.json|fixture atado a bytes del prompt y la evidencia, sin chequeo en CI
AMARILLO|agents/agent-reporter/tests/test_s3_adapters.py|prueba minio solo cubre el store y queda deseleccionada en CA-1
AMARILLO|agents/agent-reporter/src/agent_reporter/s3.py|variable `e` sin usar; CA-3 no limpia sus objetos; CA-4 no pegado en la bitácora
INFORME: revisiones/U8-T05/ronda-1.md
