# Ronda 2 — U8-T07

VEREDICTO: VERDE

Worktree `/home/omarjayg/Javeriana/topicos-especiales/agentic-qa-swarm-wt-U8-T07`, sha fd386cb8bf2690da1abbb5fbbfe5968bc407ee1a. Quedó limpio tras mis corridas (`git status --short | wc -l` = 0).

El diff 4148bd8..fd386cb toca `scripts/secrets/generate.sh`, `scripts/test/secrets-e2e.sh`, `docs/operaciones/secrets.md`, la bitácora y el informe de la ronda 1. No toca `deploy/` ni `scripts/kind/`, así que CA-2 y CA-3 quedan por el resultado de la ronda 1. Los verifiqué entonces con ejecución propia (`go-intake igual`, `ui-api igual`; `deploy rc=0` y 0 FALLA). Esta ronda no los repetí. Corrí lo que pudo cambiar: CA-1, CA-4 y `secrets-e2e.sh`.

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Variables en base y fuera del overlay | los dos `grep -c` de la tarea | pasa: `3` y `0` |
| 2 | Mismo entorno efectivo en kind | `envde` con merge-base (ronda 1) | pasa en la ronda 1. `deploy/` sin diff desde entonces |
| 3 | Plataforma sana en kind | bloque bajo `flock` (ronda 1) | pasa en la ronda 1: `deploy rc=0`, 0 FALLA. Sin cambios en `deploy/` ni `scripts/kind/`, no lo repetí |
| 4 | Políticas, secretos y alcance | `policies.sh \| grep -c '^FALLA'`; `git status --short \| wc -l`; `git diff --name-only $b \| grep -v <patrón con scripts/test/secrets-e2e.sh>` | pasa: `0`, `0`, `0` |
| Extra | `scripts/test/secrets-e2e.sh` | `TMPDIR=<scratchpad> bash scripts/test/secrets-e2e.sh` | rc=0, `E2E OK`, 0 `[DIFIERE]`. Salida en `.../scratchpad/e2e2.out` |

## Hallazgos de la ronda 1, uno por uno
- **F-01 (NARANJA, clave del LLM): cerrado.**
  - `generate.sh` exige `AGENTS_LLM_API_KEY` junto a las `BACKUP_*` y emite `agents-llm-key` en `aqs-system` con clave `api-key`. El valor lo da el humano, no se genera.
  - Las pruebas del e2e confirman el contrato: `aqs-system/agents-llm-key:api-key` está en la lista, y sin la variable el script sale con rc=2 (`[ok] sin AGENTS_LLM_API_KEY (F-01)`).
  - Lectura de vuelta que hice yo: generé con `AGENTS_LLM_API_KEY="sk-it's:a#b"` y descifré con sops. Devuelve `sk-it's:a#b` exacto, así que el escapado de la comilla simple es correcto. El fichero cifrado no contiene `sk-it` en claro (`grep -c` = 0).
  - `secrets.md` documenta el Secret y la variable, y el párrafo «Pendiente» ya no está.
  - No se monta nada, como corresponde: eso es de U8-T05.
- **F-02 (NARANJA, `secrets-e2e.sh` roto): cerrado.**
  - Expectativas actualizadas a 10 archivos, contrato con los 10 Secrets y 10 Secrets en build.
  - Se añadió la negativa sin la variable.
  - La corrida es completa (pulls de imágenes, `generate.sh` real, sops, kustomize, kubeconform), no una suite instantánea.
  - El cambio en `scripts/test/secrets-e2e.sh` encaja con el patrón de CA-4 ampliado por el arbitraje.
- **F-03 (AMARILLO, evidencia resumida): cerrado.** La bitácora de la ronda 2 trae comandos, salidas, rutas de log y el intento fallido de `kind-up` declarado como no válido. Es honesto y suficiente.
- **F-04 (AMARILLO, prerrequisitos de `ui-api`): cerrado.** `secrets.md` declara `go-warm-manager-service-token` y `go-identity-users` como prerrequisitos de `ui-api` en dev y prod.

## Hallazgos nuevos
Ninguno ROJO ni NARANJA.

## Tareas candidatas (defectos reales fuera de alcance)
- `kind-up.sh` falla de forma intermitente con `could not find a log line ... Multi-User System` (visto en mi primer intento de la ronda 1 y en el de la ronda 2 del codificador), con los clústeres `ckad` y `aqs-poc` activos en la máquina. Conviene documentarlo o añadir un reintento. Ya lo señalé en la ronda 1.

## Transcripciones
- `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/3e524589-8d03-4b3f-afd8-e3852d59d1ee/scratchpad/e2e2.out`

El informe de esta ronda no está escrito en `revisiones/U8-T07/ronda-2.md`: soy de solo lectura. El orquestador debe persistir este texto en esa ruta. Solo el humano fusiona.

VEREDICTO: VERDE
INFORME: revisiones/U8-T07/ronda-2.md
