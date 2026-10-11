# Ronda 2 — U8-T04

VEREDICTO: VERDE

No pude escribir `revisiones/U8-T04/ronda-2.md` porque mis herramientas son de solo lectura. Hay que guardar este texto en esa ruta.

Revisé la rama `tarea/U8-T04` en d0012ee (igual a `origin/tarea/U8-T04`). El worktree quedó limpio tras mis ejecuciones (`git status --short` sin líneas, también dentro del bloque de clúster). Usé el clúster `aqs` en un único bloque `flock`, de kind-up a kind-down, y solo con kind-aqs.

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Pruebas de la app | `bash scripts/test/u8-target-app-local.sh` | **pasa**: 14 OK (los 10 de la ronda 1 más `db-tarde-app-sigue-viva`, `db-tarde-health-503` y `db-tarde-recupera-health-200`), `rc=0`, 45 s. El tiempo es creíble: tiene un `sleep 35` y arranca Postgres. |
| 2 | `baseline.sh` en la imagen de go-reset | `bash scripts/test/u8-target-app-local.sh --baseline-script` | **pasa**: `clean` rc 0, `verify` entero, `version` = `ref-v1`, `rc=0`. |
| 3 | Kind, clúster NUEVO, imágenes reconstruidas desde d0012ee | `deploy.sh`; etiqueta `kind-stub`; `u8-target-app-kind.sh` | **pasa** (con la errata E-1): `deploy rc=0` (`deploy: OK`), `warm-app` con `restarts=0`, etiqueta vacía (`<stub`), y el script da `rc=0` en 9 s. Salida del script: `OK reset-previo-warm-ready`, `deploy-target-app-ensure`, `deploy-target-app-aceptado`, `deploy-target-app`, `OK superficie-alcanzable`, `PENDIENTE superficie-con-orders (C-107, U8-T05)`, `OK ensuciar-base`, `OK sucio-antes-del-reset`, `OK reset-verificado`, `OK orders-vacio-tras-reset`, `OK verify-0-tras-reset`. |
| 4 | Políticas y alcance | `policies.sh \| grep -c '^FALLA'`; `git status --short \| wc -l`; fuera de alcance | **pasa**: 0, 0 y 0. |

## Verificación de los hallazgos de la ronda 1
- **F-01 (ROJO, Service sin :9090): cerrado.**
  - El Service `warm-app` expone `80 9090` en el clúster (leído con `kubectl get svc … -o jsonpath='{.spec.ports[*].port}'`).
  - `baseline.sh` lleva `wget -T "$TIMEOUT"` (10 s por defecto).
  - El reset de go-reset termina bien en el clúster nuevo.
- **F-02 (ROJO, la app sale a los 30 s sin base): cerrado.**
  - Con la arquitectura nueva la app escucha ya, `Init` reintenta con backoff y `gate` responde 503 hasta tener base.
  - En clúster nuevo `deploy.sh` da rc=0 y `restarts=0`. En la ronda 1 daba rc=1 en 2 de 2 ejecuciones.
  - La prueba local `db-tarde` demuestra que sigue viva, da 503 y se recupera a 200.
- **F-03 (spec, `superficie-con-orders`): cerrado con la errata E-1 del humano.**
  - `superficie-alcanzable` lee `/openapi.json` por `wget` desde el pod de go-warm-manager y exige `/orders` y `/products`.
  - El script imprime `PENDIENTE superficie-con-orders (C-107, U8-T05)`, igual que `kind-smoke.sh` con el aprovisionamiento MinIO.
- **F-04 (NARANJA, la prueba no ensuciaba ni leía de vuelta): cerrado. Probé que discrimina.**
  - El script ejecuta `POST /orders` (`ensuciar-base`), comprueba `verify > 0` (`sucio-antes-del-reset`), resetea y lee de vuelta `GET /orders` vacío y `verify == 0`.
  - Rojo 1: con `U8_RED_SKIP_FINAL_RESET=1`, `orders-vacio-tras-reset` y `verify-0-tras-reset` dan FALLA y `rc=1`.
  - Rojo 2, más cercano a lo que pediste ("`clean` desactivado"): sustituí en vivo el ConfigMap `go-reset-baseline` por el relleno (`clean` no hace nada, `verify` imprime 0, `version` imprime `kind-stub`), reinicié go-reset y repetí el script. Salió `OK reset-verificado`, que es el engaño del relleno, pero `FALLA orders-vacio-tras-reset` y `FALLA verify-0-tras-reset`, con `rc=1`. La prueba ya distingue el baseline real del relleno.
  - Defecto cosmético (AMARILLO): las líneas `FALLA` de esos checks imprimen la salida de diagnóstico vacía.
- **F-05 (NARANJA, modo 0444): cerrado.**
  - `warm-app` tiene `securityContext.fsGroup: 65532` en el pod (leído del clúster).
  - `defaultMode` del Secret es 288 (= 0440) en `warm-app` y en `go-reset`, leído de los Deployments. No queda 0444.
- **F-06 (AMARILLO): corregido o justificado, y la justificación es aceptable.**
  - Corregido: `kind-local.md` ahora dice 10 imágenes y reset verificado; `kind-smoke.sh` ya no dice PENDIENTE de reset; `secrets.sh` compara el token entre aqs-test y aqs-system y falla si difieren; `AQS_IMAGES` valida con `awk '$1==n'`; `constantTimeEq` se cambió por `subtle.ConstantTimeCompare`; la semilla del `Diff` sale ahora de `seedProducts`.
  - Justificado, no cambiado (todas quedan como candidatas): `sslmode=disable` heredado, exposición de `:9090` en el namespace, dev/prod sin el Secret hasta U8-T07, `GOTOOLCHAIN=auto`, postgres por tag y no por digest.
  - El humo `kind-smoke.sh` en el clúster nuevo da `smoke: OK (sin FALLA; …)`, con `reset verificado: cubierto por scripts/test/u8-target-app-kind.sh`.

## Rebase y fusión
- La rama contiene el merge de U8-T06 (78a691b). `git diff 78a691b HEAD -- deploy/flux/base/control-plane.yaml` son solo +13 líneas de U8-T04, sin tocar cambios de U8-T06.
- U8-T01 (PR #96) se fusionó mientras revisaba y `origin/main` quedó en 8357780. La rama no incluye ese commit, pero `git fetch && git merge-tree --write-tree HEAD origin/main` da **rc=0**, sin conflicto (árbol 2b556ec).
- En ese árbol fusionado, `kind-local.md` conserva a la vez la sección NATS de U8-T01 y los cambios de U8-T04 (10 imágenes, reset verificado). `scripts/kind/deploy.sh` conserva lo de NATS.
- `control-plane.yaml` fusionado difiere de `origin/main` solo en las 13 líneas de U8-T04.
- No hay conflicto que reportar.

## Alerta de seguridad: estado final
- Ninguno de los puntos (a) a (g) debilita la seguridad.
- (a) `sslmode=disable` (warm-db sin TLS), (c) `cap_net_bind_service` y (e) postgres por tag son AMARILLO, ya conocidos.
- (d) `label=disable` es solo de pruebas.
- (f) Los modos de Secret son ahora 0440.
- (g) `policy/` y las NetworkPolicy no se tocaron; `policies.sh` da 0 FALLA.

## Tareas candidatas (fuera de alcance)
- **U8-T05.** Aprovisionar el usuario MinIO `aqs-evidence` (C-107) y rehabilitar el check completo `superficie-con-orders` con `POST /surface`.
- **NetworkPolicy.** Acotar `:9090` al par go-reset → warm-app. Hoy `allow-same-namespace` de aqs-test deja que cualquier pod del namespace alcance `:9090`, con el token como única barrera.
- **go-reset.** Ejecutar el reset con un contexto desligado de la petición HTTP. La causa del estado "dirty sin cuarentena" cuando el cliente corta (ronda 1) sigue en go-reset.
- **Digest.** Fijar la imagen de postgres por digest.
- **U8-T07.** `target-app-baseline-token` para dev/prod. Hasta entonces, no aplicar `base` a un clúster Flux real.

## Rutas de transcripciones largas
- `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/1c7b66eb-a3e9-4dad-81e8-f56a81f889ed/scratchpad/u8t04-r2.out` (clúster completo: deploy, CA-3 verde, rojos, humo)
- `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/1c7b66eb-a3e9-4dad-81e8-f56a81f889ed/scratchpad/u8t04-r2.sh` (script que ejecutó el bloque)

VEREDICTO: VERDE
AMARILLO|scripts/test/u8-target-app-kind.sh|Los FALLA de orders-vacio-tras-reset y verify-0-tras-reset no imprimen diagnóstico (cosmético)
INFORME: revisiones/U8-T04/ronda-2.md
