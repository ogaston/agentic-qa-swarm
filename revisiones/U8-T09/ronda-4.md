# Ronda 4 — U8-T09

VEREDICTO: NO-VERDE

Corrí yo mismo CA-1, CA-2 y CA-4 con los comandos de la tarea, más las unitarias de planner y reporter. Todo pasa. El único hallazgo bloqueante es documental (NARANJA, F-01): la doc operativa afirma lo contrario de lo que hace el código. El arreglo es barato y la guarda http sale bien parada.

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Sin claves en git ni en logs; planner y reporter con LLM real | `git grep -c -i -E 'sk-[a-z0-9]{20,}' \| wc -l; bash scripts/kind/kind-smoke.sh 2>&1 \| grep -E '^(OK\|FALLA) (secrets-no-en-logs\|llm-real-configurado)'` | pasa: `0`, `OK secrets-no-en-logs`, `OK llm-real-configurado`. El smoke completo dio rc=0, 0 FALLA y `OK llm-egress-acotado`. |
| 2 | Ciclo completo con modelo real | `bash scripts/kind/kind-e2e.sh --bug off 2>&1 \| grep -c '^FALLA'; echo rc=${PIPESTATUS[0]}`. Para ver los pasos lo redirigí a un archivo (`$S/e2e.txt`, `$S` = scratchpad de la sesión) con rc=0. | pasa: `0` y `rc=0`. 20 OK, incluidos `ensayo-job`, `runners`, `reset-verificado`, `run-terminado`, `reporte-en-minio` y `veredicto-sin-hallazgos`. Los pasos `planner-fixture` y `reporter-fixture` salen como «omitido» (no cuentan como OK). El planner registra `reason=ok` en las corridas. |
| 3 | Webhook real de GitHub | No lo cuento como fallo del codificador (depende del humano). Corrí el script con 3 SHA distintos. | Negativos: `zzz` da rc=2 y un SHA inexistente (timeout 8 s) da rc=1 con `FALLA`. Positivo: con el SHA de una notificación que el e2e dejó en el inbox da `OK webhook-github-recibido` y rc=0. La lectura es del inbox real vía port-forward y login, y el script cierra sus port-forwards. |
| 4 | Alcance | `git status --short \| wc -l` y el `git diff --name-only $b \| grep -v -E '…' \| wc -l` de la tarea | pasa: `0` y `0`. 17 archivos en el diff, todos dentro de la lista ampliada. |

Unitarias, con venvs propios en el scratchpad y `-p no:cacheprovider`:
- agent-planner: 212 passed, 7 deselected.
- agent-reporter: 299 passed, 3 deselected. El reporter necesita `requirements.txt` además de `requirements-dev.txt` (falta `botocore`); es una nota, no un fallo.

El worktree quedó limpio tras mi revisión (`git status --short | wc -l` = 0). No quedan port-forwards míos.

## Guarda http (decisión del humano, comprobada)
La guarda sigue acotada. Sondeé `check_base_url` del planner con 15 casos:
- Sin `LLM_ALLOW_INSECURE_HTTP=1` exacto o con host vacío o `None`: rechaza.
- Si el host de la URL no es el configurado: rechaza. Con `user@` en la URL, el hostname queda como el host real.
- IP pública, nombre DNS, `*.nip.io`, IPv6 mapeado a una IP pública y octetos con ceros a la izquierda: rechaza.
- 100.64.0.1: acepta solo con el host exacto. Los límites 100.63.255.255 y 100.128.0.0 están cubiertos por tests y rechazan.
- `base/dev/prod` no definen las variables: la excepción solo existe en el overlay de kind (`grep` sobre `deploy/` lo confirma).
- No hay redirects: se mantiene `_NoRedirect`.
- Planner y reporter llevan código duplicado idéntico, y ambos tienen pruebas de rechazo.

No imprimí la IP, el puerto ni la clave. Comprobé sin mostrarlos que no están en git (`git grep` sobre `HEAD` y diff de la rama), ni en los logs de planner y reporter. El nombre del modelo aparece solo en el archivo de la tarea, donde el propio texto de la tarea lo declara.

## Hallazgos
### F-01 · NARANJA · docs/operaciones/kind-local.md (líneas ~64, 107, 123-124, 218-228) y deploy/flux/kind/kustomization.yaml:~248-252 · Documentación y comentarios desfasados respecto al estado final
Barrido de clase: son afirmaciones falsas heredadas de rondas anteriores, que contradicen la sección nueva del mismo archivo.
- La tabla (fila «Planner y reporter en el clúster») dice que `kind-e2e.sh` está «bloqueado por `check_base_url`, ver la sección». Falso: CA-2 pasa en esta ronda.
- La sección «Agentes en el clúster (U8-T05)» dice que el overlay de kind pone `LLM_PROVIDER=fake` con `PLANNER_ALLOW_FAKE`/`REPORTER_ALLOW_FAKE`, con ConfigMaps de fixtures que no se montan. Falso: `kubectl get` y el kustomization muestran `LLM_PROVIDER=http`.
- Las líneas 107 y 123-124 (guion del e2e) siguen describiendo la pausa del controlador para generar la fixture del planner. Con LLM real el script la omite, y la doc no lo dice.
- El comentario del kustomization dice «Upstream http: agents/*/llm_http.py lo rechaza (solo https o loopback)». Es lo que resolvió la excepción de la ronda 4.

Un operador que siga la doc se confunde sobre el estado real del entregable. Arreglo: actualizar esas frases y marcar qué parte del guion de la demo depende de las fixtures falsas.

### F-02 · AMARILLO · tarea (alcance) / docs/operaciones/kind-local.md · Costo de tokens sin medir
El alcance dice «se mide y se anota el costo de tokens de una corrida de demo (P4 queda candidata)». La bitácora y la doc dicen que no hay dato, porque los agentes no persisten el `usage`. La razón es verosímil y queda como candidata, pero el humano debe aceptar explícitamente esta omisión al fusionar. No bloquea.

### F-03 · AMARILLO · agents/*/src/*/llm_http.py:_is_private_ip_literal · `ip.is_private` es más ancho que «red privada»
Acepta, si el operador lo configura como host exacto, `169.254.169.254` (metadatos de nube), `0.0.0.0` y `fd00::/8`. El host sale de un Secret local, no de la entrada, así que no es explotable hoy. Endurecerlo (excluir link-local y unspecified) sería barato, y le falta un test que fije el comportamiento. Además no hay caso de test para `LLM_ALLOW_INSECURE_HTTP=1` con host vacío (lo comprobé manualmente y rechaza).

### F-04 · AMARILLO · scripts/kind/github-webhook-check.sh · Modo 0644
El archivo no es ejecutable (git ls-files: 100644), aunque la doc y el CA lo invocan con `bash`, así que funciona. Otros scripts de `scripts/kind/` también son 0644, pero este es entregable nuevo: `chmod +x` es trivial. La doc del webhook y el script son correctos contra el código real: puertos 18625/18626, ruta `/webhooks/github`, campos del inbox, y el Secret `go-intake-webhook` con clave `secret`.

## Tareas candidatas (fuera de alcance, no exijo arreglo aquí)
- `scripts/test/u8-agents-kind.sh` asume LLM falso y el venv local. Contra el overlay de kind actual (LLM real) quedaría roto; es un test fuera de la lista de CA-4.
- `deploy/flux/base/agents.yaml` conserva `LLM_BASE_URL=http://litellm...` y egreso a un pod que no existe (la bitácora ya lo anota).
- Persistir `usage` de tokens en los agentes (P4).
- `kind-up.sh`: el `wait` sobre nodos da Forbidden con este kubeconfig, y el lock `aqs-kind.lock` lo retiene un fd heredado del bringup (la bitácora ya lo anota).
- Resiliencia del LLM tras un 401 del upstream (cooldown y `llm_unavailable`).

## Rutas de transcripciones largas
- /tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/71dd2975-7e45-44f4-8b4c-80be75f11bbd/scratchpad/e2e.txt
- /tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/71dd2975-7e45-44f4-8b4c-80be75f11bbd/scratchpad/smoke.txt

VEREDICTO: NO-VERDE
NARANJA|docs/operaciones/kind-local.md y deploy/flux/kind/kustomization.yaml (desfase con el estado final)|Documentación y comentarios afirman que el e2e está bloqueado y que el overlay usa LLM falso
