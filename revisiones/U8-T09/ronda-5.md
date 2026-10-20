# Ronda 5 — U8-T09 (HEAD e48637c, código en c1162ba)

VEREDICTO: VERDE

F-01, F-03 y F-04 están resueltos y F-02 queda documentado como candidata P4, a aceptar por el humano al fusionar. Con las imágenes de HEAD reconstruidas, corrí yo mismo CA-1, CA-2, CA-4 y las unitarias, y todo pasa. No queda ningún ROJO ni NARANJA.

## Criterios, verificados por mí
| # | Comando | Resultado |
|---|---|---|
| CA-1 | `git grep -c -i -E 'sk-[a-z0-9]{20,}' \| wc -l`; `bash scripts/kind/kind-smoke.sh 2>&1 \| grep -E '^(OK\|FALLA) (secrets-no-en-logs\|llm-real-configurado)'` | pasa: `0`, `OK secrets-no-en-logs`, `OK llm-real-configurado` |
| CA-2 | `bash scripts/kind/kind-e2e.sh --bug off` | pasa: rc=0, 0 FALLA, 20 OK. Incluye `ensayo-job`, `runners`, `reset-verificado`, `run-terminado`, `reporte-en-minio` y `veredicto-sin-hallazgos`. |
| CA-4 | `git status --short \| wc -l`; `git diff --name-only $b \| grep -v …` | pasa: `0` y `0` |
| Unitarias | pytest en cada agente, con venvs propios en el scratchpad | planner 226 passed; reporter 312 passed |

- **CA-2:** decidí repetirlo porque los pods corrían imágenes anteriores al cambio de la guarda y la evidencia no podía venir de c1162ba. Antes comprobé con `pgrep` que no había otros scripts del clúster en curso. Reconstruí e instalé planner y reporter con `build-images.sh` y `load-images.sh`, y reinicié ambos Deployments. Los dos rolled out. El e2e dejó en `$S/e2e5.txt` (`$S` = scratchpad de la sesión): `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/71dd2975-7e45-44f4-8b4c-80be75f11bbd/scratchpad/e2e5.txt`.
- **CA-3:** no lo conté, como pediste. Los chequeos de la ronda 4 del script de webhook y su documentación siguen vigentes.
- **Worktree:** limpio tras mi revisión (0 líneas).

## Hallazgos de la ronda 4
- **F-01 (NARANJA): resuelto.** En `docs/operaciones/kind-local.md`:
  - La tabla ya no dice «bloqueado» y refleja que el e2e pasa con LLM real.
  - La sección «LLM en kind» ahora dice `LLM_PROVIDER=http`, ConfigMaps de fixtures sin montar, y qué depende de fixtures falsas: solo `scripts/test/u8-agents-kind.sh` y los pasos `planner-fixture` y `reporter-fixture`.
  - La ventana del planner y la restauración de fixtures quedaron condicionadas al LLM falso.
  - En `kustomization.yaml` los comentarios de la excepción http y de los ConfigMaps quedaron actualizados.
- **F-02 (AMARILLO): documentado.** Hay un párrafo «Costo en tokens (no medido)» en `kind-local.md`, con P4 como candidata sujeta a la aceptación del humano. La medición sigue sin existir, así que la fusión debe aceptarla de forma explícita.
- **F-03 (AMARILLO): resuelto en planner y reporter.** `_is_private_ip_literal` ahora rechaza link-local, no especificada, multicast y loopback.
  - Sondeé el código nuevo: rechaza `169.254.169.254`, `fe80::1`, `0.0.0.0`, `::`, `::ffff:169.254.169.254` y un host vacío.
  - La URL real del gateway, leída del Secret `llm-upstream` sin imprimirla, sigue aceptada.
  - Hay pruebas nuevas para los casos de rechazo, para RFC 1918, ULA y CGNAT que se mantienen, y para el host vacío o `None` (también en `test_config.py` del planner).
  - La guarda sigue acotada: solo http con bandera exacta y host exacto; fuera de eso exige https o loopback.
- **F-04 (AMARILLO): resuelto.** `git ls-files -s` muestra `100755` y el archivo es `-rwxr-xr-x`.

## Hallazgos nuevos
- **AMARILLO, `docs/operaciones/kind-local.md`.** Quedan dos frases casi duplicadas sobre tokens: «Costo en tokens (no medido)» y «La medición de tokens por corrida no es posible…» en «Limitaciones conocidas». No es contradicción, solo redundancia.

## Tareas candidatas (sin cambios)
- P4: persistir `usage` de tokens.
- `scripts/test/u8-agents-kind.sh` asume LLM falso.
- `deploy/flux/base/agents.yaml` conserva `LLM_BASE_URL=http://litellm...`.
- `kind-up.sh`: el `wait` sobre nodos da Forbidden con este kubeconfig, y el lock `aqs-kind.lock` lo retiene un fd heredado.
- Resiliencia del LLM tras un 401 del upstream.

No imprimí la IP, el puerto ni la clave.

VEREDICTO: VERDE
