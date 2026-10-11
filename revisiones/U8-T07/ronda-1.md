# Ronda 1 — U8-T07

VEREDICTO: NO-VERDE

Worktree `/home/omarjayg/Javeriana/topicos-especiales/agentic-qa-swarm-wt-U8-T07`, sha 4148bd879215507900fcf03affa2c9b9da876656, limpio al terminar (0 líneas en `git status --short`).

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Variables en base, fuera del overlay | los dos `grep -c` de la tarea sobre `dev` y `kind` | pasa: `3` y `0`. En `prod` también da `3`. |
| 2 | Mismo entorno efectivo en kind | `envde` de la tarea con worktree en el merge-base 5a098ec | pasa: `go-intake igual`, `ui-api igual`. Comprobé además volúmenes y volumeMounts de ui-api: iguales. El diff del render completo de kind solo muestra reordenación de env. |
| 3 | Plataforma en kind sana | un bloque `kind-up`, `load-images`, `deploy.sh`, `kind-smoke.sh`, lectura de vuelta y `kind-down` bajo `flock` (fd 9 sobre `${XDG_RUNTIME_DIR:-/tmp}/aqs-kind.lock`) | pasa en el 2.º intento: `deploy rc=0`, `FALLA` = 0, smoke `OK` sin FALLA (3 PENDIENTE declarados). El 1.er intento falló en `kind-up` por un error del entorno (`could not find a log line ... Multi-User System`) y no llegó a deploy. Lo repetí sin tocar nada y pasó. |
| 4 | Políticas, secretos, alcance | `policies.sh \| grep -c '^FALLA'`; `git status --short \| wc -l`; `git diff --name-only $b \| grep -v <patrón> \| wc -l` | pasa: `0`, `0`, `0` |

Lectura de vuelta en el clúster, durante CA-3:
- `go-intake` y `ui-api` tienen 1 réplica disponible.
- `ui-api` tiene `UIAPI_AUTH=identity`, `IDENTITY_URL`, `WARM_URL` y `UIAPI_WARM_TOKEN_FILE=/etc/aqs/warm/token`.
- `go-intake` tiene `GITHUB_WEBHOOK_SECRET` desde `{"key":"secret","name":"go-intake-webhook"}`.

Rojo primero: la bitácora lo muestra (`0` y `3` antes del cambio).

Sobre el bloque `flock` del codificador: la bitácora solo resume el bloque (`kind-up`, `load-images`, `deploy`, `smoke`, `kind-down`, todos rc=0) y no pega el comando literal. Por esa descripción es equivalente a CA-3 (`deploy.sh` más `kind-smoke.sh` con el recuento de `FALLA`), y mi ejecución del mismo bloque reproduce el resultado. Ver F-03 (AMARILLO).

## Hallazgos
### F-01 · NARANJA · alcance «Dentro» · Falta la clave del LLM en `generate.sh` y `secrets.md`
- El alcance de la tarea dice «añadir los Secrets nuevos (`go-intake-webhook`, `target-app-baseline-token`, **la clave del LLM**) a `scripts/secrets/generate.sh` y `docs/operaciones/secrets.md`».
- Con el nombre que fijó el orquestador, ya no hay bloqueo, así que su ausencia incumple el alcance de T07. El diff no contiene `agents-llm-key`.
- `secrets.md` dice «Pendiente: ... la fija U8-T09», lo cual queda obsoleto.
- A hacer:
  - Secret `agents-llm-key` en `aqs-system`, clave de datos `api-key`, con su entrada documentada para dev y prod en `secrets.md`.
  - En `generate.sh`: el valor lo aporta el humano (variable de entorno, como `BACKUP_*`) y no se genera aleatorio. Una clave de proveedor externo no se puede inventar con `openssl rand`.
  - Con la variable ausente el script debe fallar, igual que las `BACKUP_*`.
  - Quitar el párrafo «Pendiente».
- No monta nada: eso es de U8-T05.
- El codificador actuó bien al no inventar el nombre y dejarlo como bloqueo. El hallazgo es por el alcance incompleto, no por conducta.

### F-02 · NARANJA · `scripts/test/secrets-e2e.sh:38,39,56` · El cambio de `generate.sh` rompe la prueba existente de U5-T14 y no se advirtió
- Corrí `bash scripts/test/secrets-e2e.sh` (rc=1; salida en `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/3e524589-8d03-4b3f-afd8-e3852d59d1ee/scratchpad/e2e.out`). Líneas relevantes:
  - `[DIFIERE] 6 archivos: obtenido='9' esperado='6'`
  - `[DIFIERE] contrato de Secrets` (faltan `go-intake-webhook` y los dos `target-app-baseline-token`)
  - `[DIFIERE] Secrets en build (esperado 6): obtenido='9'`
- Hay que actualizar sus expectativas: 9 archivos, el contrato con los 3 Secrets nuevos (y `agents-llm-key` cuando F-01 se cierre) y el recuento de Secrets en build.
- La bitácora no menciona la prueba y declara que `generate.sh` «no se ejecutó de extremo a extremo»; esa prueba sí lo ejecuta.
- Conflicto de especificación: `scripts/test/` no está en el patrón de CA-4 (`git diff --name-only ... | grep -v`). Corregirla hoy da `1` en el tercer comando de CA-4. El orquestador debe ampliar el patrón con `scripts/test/secrets-e2e.sh` para que el arreglo no cuente como desborde. Si no se amplía, T07 no puede quedar verde sin romper la prueba.
- Lo que sí está bien: `generate.sh dev|prod` corre de extremo a extremo con `rc=0` y emite 9 Secrets cifrados (`OK: 9 Secrets cifrados`). `ENC[` está presente, no hay valores en claro, `check-secrets.sh` da OK y `policies.sh` sobre lo generado da `rc=0`. Eso es evidencia positiva del cambio, no solo `bash -n`.

### F-03 · AMARILLO · `bitacoras/U8-T07.md` · Evidencia de CA-3 resumida, sin comando literal ni salida pegada
La bitácora da rc y recuentos en prosa, no el bloque `flock` ni la salida. Es verificable porque reproduje CA-3, pero la próxima ronda debería pegar comando y salida. Lo mismo en la ronda 2 para el arreglo de F-01 y F-02: pegar la corrida de `secrets-e2e.sh`.

### F-04 · AMARILLO · `docs/operaciones/secrets.md` · Texto menor
- Hay que mencionar que `go-warm-manager-service-token` y `go-identity-users` (sección 6) son prerrequisito de `ui-api` en dev y prod. Hoy `ui-api` los exige en base y el documento solo lo dice de forma indirecta.
- Los Secrets de dev y prod que el generador crea no están en `deploy/flux/{dev,prod}/secrets/`. Eso es correcto según la tarea («solo plantillas y documentación»).

## Tareas candidatas (defectos reales fuera de alcance)
- `kind-up.sh` falló una vez con `could not find a log line ... Multi-User System` aunque no existía el clúster `aqs`. Puede ser contención con los clústeres `ckad` y `aqs-poc` activos en la máquina (límites de inotify o de recursos en podman). Conviene que la preparación de la máquina para CA-3 lo documente o añada un reintento.

## Rutas de transcripciones largas
- `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/3e524589-8d03-4b3f-afd8-e3852d59d1ee/scratchpad/e2e.out` (`secrets-e2e.sh`)
- `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/3e524589-8d03-4b3f-afd8-e3852d59d1ee/scratchpad/smoke.out` (humo)

El informe no se escribió en `revisiones/U8-T07/ronda-1.md` porque mi rol es de solo lectura. El orquestador debe persistir este texto en esa ruta.

VEREDICTO: NO-VERDE
NARANJA|alcance «Dentro» (generate.sh y secrets.md)|Falta la clave del LLM (`agents-llm-key` / `api-key`) en generate.sh y secrets.md
NARANJA|scripts/test/secrets-e2e.sh:38,39,56|El cambio de generate.sh rompe secrets-e2e.sh (6 vs 9 Secrets); el patrón de CA-4 no cubre scripts/test
AMARILLO|bitacoras/U8-T07.md|Evidencia de CA-3 resumida, sin comando literal ni salida
AMARILLO|docs/operaciones/secrets.md|Dejar explícitos los prerrequisitos de ui-api en dev y prod
INFORME: revisiones/U8-T07/ronda-1.md
