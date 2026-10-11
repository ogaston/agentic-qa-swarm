# Ronda 2 — U7-T04 (sha cb4fc32)

VEREDICTO: VERDE

Corrí los 5 CA yo mismo y pasan. F-01 a F-04 quedan cerrados con lectura de vuelta, y no queda ningún ROJO ni NARANJA. Para esta ronda levanté el clúster otra vez (`kind-up`, `build-images`, `load-images`, `deploy`); `kind-up` funcionó al primer intento, sin el fallo transitorio. Usé solo `kind-aqs`. Al terminar corrí `kind-down`, y `kind get clusters` muestra `aqs-poc` y `ckad` intactos. El contexto quedó en `kind-ckad`, el worktree está limpio (`git status --short | wc -l` da 0) y no quedan port-forwards ni directorios `aqs-smoke.*`.

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Humo completo | El de la tarea, literal | pasa: `22 OK`, ninguna `FALLA`, `rc=0` |
| 2 | Sensibilidad de red | El de la tarea, literal | pasa: `1` y `1` |
| 3 | Nada colgado | El de la tarea, literal | pasa: `0` y `0` |
| 4 | Pendientes y runbook | El de la tarea, literal | pasa: `2` y `8` |
| 5 | Alcance | El de la tarea con `git merge-base HEAD origin/main` | pasa: `0` y `0` |

Más datos de la ejecución:
- **CA-2:** sin políticas, la línea pasa a `FALLA netpol-sin-egress-test (testigo interno alcanzable: kubernetes.default 10.96.0.1:443)`. Repuestas las 4 políticas, vuelve a `OK`.
- **CA-5:** el merge-base sigue en `acd980a`. El diff toca solo `bitacoras/U7-T04.md`, `deploy/flux/kind/kustomization.yaml`, `docs/operaciones/kind-local.md`, `revisiones/U7-T04/ronda-1.md` y `scripts/kind/kind-smoke.sh`. Con `git diff --stat` sobre `deploy/flux/base`, `dev`, `prod` y `policy` no sale ninguna línea.
- **`policies.sh`:** 0 `FALLA`.
- **Pendientes:** el reset verificado sigue como `PENDIENTE` por C-104. Ninguna comprobación se apoya en `kind-stub`.

## Cierre de hallazgos, con lectura de vuelta
- **F-01 (red sin depender de Internet): cerrado.**
  - Con las políticas borradas, la `FALLA` nombra el testigo interno. El texto del script ya no incluye `go-identity`.
  - Simulé la falta de Internet. Borré las políticas de `aqs-test` y apliqué una NetworkPolicy que permite todo el egress salvo `1.1.1.1/32`. El resultado fue `OK netpol-control-positivo` y `FALLA netpol-sin-egress-test (testigo interno alcanzable: kubernetes.default 10.96.0.1:443)`. Esa línea solo lista `kubernetes.default`, sin `1.1.1.1`, así que sin salida a Internet el aislamiento sigue detectándose. Quité mi política de prueba y repuse las 4 originales.
- **F-02 (runbook): cerrado.**
  - La fila de `warm-estado` dice ahora «Funciona. Solo en kind: el overlay define `WARM_URL`… E-3… en base/dev/prod el hueco sigue abierto (C-103)». `grep -i 'bloqueado|FALLA, defecto'` sobre el runbook no devuelve nada.
  - El runbook explica por qué `go-identity` no es testigo y que el sensor interno no depende de Internet.
- **F-03 (señales durante la limpieza): cerrado.**
  - Mandé 3 `TERM` seguidos a los 12 s, con el pod de prueba y los port-forwards activos. Salió con `rc=130`.
  - Después no había directorio `aqs-smoke.*`, ni pod de prueba, ni puertos en 186xx.
  - En ronda 1 el mismo tipo de prueba dejó el directorio temporal con la contraseña demo y los tokens.
  - Un intento mío de repetir la prueba con `INT` no sirve como evidencia. Las señales llegaron a los 20 s, cuando el script ya había terminado solo: salió `rc=0` y la salida acaba en `smoke: OK`. En un subshell en segundo plano bash ignora además `SIGINT`. No es un defecto del script.
- **F-04 (logs vacíos): cerrado.**
  - Probé con un `kubectl` de sustitución en el `PATH` que devuelve vacío para `logs`. El script imprime `FALLA secrets-no-en-logs` con «los logs de los 7 Deployments están vacíos (0 líneas leídas)».
  - Sin el sustituto, la misma comprobación da `OK`.

## Hallazgos en pie
Ninguno ROJO ni NARANJA. No añado AMARILLOS.

## Tareas candidatas
- Función compartida de testigos de red en `scripts/kind/lib.sh`, ya aceptada por el orquestador como candidata y fuera de esta ronda.

## Rutas de transcripciones largas
Salidas de mi corrida, en `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/0ecca73d-838d-45a3-8b36-868c95cd55c0/scratchpad/`:
- `smoke-nonp2.out`
- `smoke-offline.out`
- `deploy2.out`

VEREDICTO: VERDE
INFORME: revisiones/U7-T04/ronda-2.md
