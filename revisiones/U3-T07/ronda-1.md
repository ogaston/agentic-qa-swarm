# Ronda 1 — U3-T07

VEREDICTO: VERDE

Corrí yo mismo CA-1 a CA-5 en `/home/omarjayg/Javeriana/topicos-especiales/agentic-qa-swarm-wt-U3-T07` (rama `tarea/U3-T07`, HEAD `ba494d8`). Hice `git fetch` antes. La base de CA-5 es `8ca8d13`, el merge-base con `origin/main`. Antes de empezar, los puertos 18400 y 18401 estaban libres y el worktree estaba limpio.

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Suites en verde y eval | Los 3 `pytest -m 'not pbt_demo'` y `agent_eval run` | pasa: planner `184 passed`, reporter `274 passed`, eval `102 passed`, `eval rc=0` |
| 2 | Adaptador LLM | `pytest -k llm_http -v \| grep -c PASSED` en planner y reporter | pasa: `11` y `11` (mínimo exigido: 6) |
| 3 | Arranque y parada | `u3-up.sh`, un `curl` por URL, `u3-down.sh`, `curl --max-time 2` | pasa: `200`, `200` y tras parar `000` |
| 4 | Recorrido completo | `u3-demo-local.sh \| grep -E '^(OK\|FALLA)'` | pasa: 7 líneas `OK`, ninguna `FALLA`, `rc=0` |
| 5 | Higiene y alcance | grep de SDKs, `git status --short \| wc -l`, `git diff --name-only $b` filtrado | pasa: `0`, `0`, `0` |

Detalles de CA-3, CA-4 y CA-5:
- **CA-3:** apliqué la decisión 4 y llamé a `curl` una vez por URL.
- **CA-4:** las 7 líneas `OK` fueron `u3-up`, `contrato-u2-plan-valido`, `contrato-u2-plan-invalido`, `contrato-u2-u3-detenido`, `reporte-valido`, `reporte-con-evidencia` y `u3-detenido-limpio`. Al terminar, `ss -ltn` no muestra nada en 18400/18401 y el directorio de estado fue borrado.
- **CA-5:** la salida de `git diff --name-only` filtrado con el patrón de la tarea quedó vacía.

## Comprobaciones adicionales
- **Contrato U2 no vacío:** corrí `go test -v -tags contract -run 'Contract(U3)'` con `U3_URL` apuntando al planner real. Pasan `TestContractU3ValidFlowPlan`, `TestContractU3InvalidResponseFailsPhase` y `TestContractU3StoppedStubFailsPhase`, sin `SKIP`.
- **Reporter real:** `POST /v1/report` con la evidencia sembrada devuelve un reporte con `verdict: bug`. Cada `evidence_uris` del hallazgo está dentro de la petición.
- **Adaptador `llm_http.py`:**
  - Usa solo la biblioteca estándar y no hace reintentos.
  - No sigue redirecciones, así que la clave no viaja a otro destino.
  - Los errores llevan un mensaje fijo y `from None`, sin cuerpo de respuesta ni clave.
  - `http` se acepta solo hacia loopback, y `urlsplit(...).hostname` se encarga de URLs con userinfo.
  - La copia del reporter solo difiere en el docstring.
- **Cableado:** cualquier `LLM_PROVIDER` distinto de `fake` o `http` sigue sin arrancar. `fake` sigue exigiendo `*_ALLOW_FAKE` y rechazando `prod`.
- **Worktree:** limpio tras las revisiones.

## Hallazgos
No hay ROJO ni NARANJA.

### F-01 · AMARILLO · `scripts/test/u3-up.sh` y `u3-down.sh` · pid en archivo
`u3-down` mata el pid guardado en `$state/*.pid` sin comprobar que siga siendo el proceso propio. Si el pid se reutilizara tras un crash, podría matar otro proceso. No lo veo bloqueante: el estado se borra en cada parada y la tarea es de uso local.

### F-02 · AMARILLO · `agents/*/src/*/llm_http.py` · `usage` obligatorio
Un servidor compatible con OpenAI que omita `usage` produce `LLMUnavailable("respuesta_sin_campos")` aunque haya contenido. Es coherente con «los tokens de `usage`» de la tarea y falla cerrado. Se podría documentar o tolerar en una tarea posterior.

### F-03 · AMARILLO · `reporte-con-evidencia` · prueba parcialmente tautológica
Con el `FakeLLM` la respuesta del reporter es una fixture, así que la comprobación valida el cableado y el esquema, no la correlación del modelo. Es inherente al alcance de la tarea (sin proveedor real) y no bloquea.

## Tareas candidatas
- Probar manualmente contra DeepSeek/LiteLLM (paso del humano, ya anotado en las notas de la tarea).
- Eventualmente consolidar la duplicación de `llm_http.py` (planner y reporter) en una librería compartida, cuando se reactive el backlog post-MVP.

## Rutas de transcripciones largas
Ninguna. Todas las salidas eran cortas y están citadas arriba.

VEREDICTO: VERDE
AMARILLO|scripts/test/u3-down.sh|pid sin verificación de propiedad del proceso (no bloquea)
INFORME: revisiones/U3-T07/ronda-1.md
