# Ronda 2 — U5-T15

VEREDICTO: VERDE

Hash de la tarea: 5cec844b357aafbc30689deb0cd7f546041d8de8 (coincide). HEAD del worktree: 2685f42. Worktree limpio antes y después de revisar (`git status --short | wc -l` da `0`). Todo lo corrí yo, sobre copias de `git archive HEAD` en `mktemp -d`. No toqué GitHub.

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | El script existe y pasa | `test -x scripts/ci/policies.sh && bash scripts/ci/policies.sh >/dev/null 2>&1; echo rc=$?` | pasa: `rc=0` |
| 2 | 5 comprobaciones en ambos overlays | el `grep -c -E` literal | pasa: `8`. Las 8 líneas son `OK`. |
| 3 | 4 negativas | `neg()` literal ×4 | pasa: politica `rc=1`, default-deny `rc=1`, kubeconform `rc=1`, promtool `rc=1` |
| 4 | Workflow | actionlint 1.7.7 y los 4 `grep` | pasa: `rc=0`, luego `2`, `1`, `0`, `1` |
| 5 | shellcheck, alcance, árbol limpio | shellcheck v0.10.0, `git diff --name-only`, `git status --short \| wc -l` | pasa: `rc=0`; exactamente `bitacoras/U5-T15.md`, `.github/workflows/policies.yml` y `scripts/ci/policies.sh`; y `0` |

## 1. Mis casos fail-open de la ronda 1, repetidos
| Caso | Resultado |
|---|---|
| Overlay dev con `resources: []` | `FALLA kustomize-build dev (salida vacia o sin documentos con kind)`; luego `FALLA kubeconform dev`, `FALLA conftest-test dev` y `FALLA conftest-combine dev`. prod sigue y da `OK`. `rc=1`. |
| Overlay prod con `resources: []` | `FALLA kustomize-build prod`, las 3 comprobaciones de prod en `FALLA`, y también `FALLA promtool-rules` porque depende del build de prod. `rc=1`. |
| Ambos overlays con `resources: []` | Todo `FALLA` salvo `conftest-verify`. `rc=1`. |
| `rules_test.yaml` con `tests: []` | `FALLA promtool-rules`; el resto `OK`. `rc=1`. |
| `rules_test.yaml` de 0 bytes | `FALLA promtool-rules`. `rc=1`. |
| `policy/` borrado | `FALLA` en conftest-test ×2, conftest-combine ×2 y conftest-verify. `rc=1`. |
| `policy/` sin ningún `.rego` | Igual que el anterior. `rc=1`. |
| `kustomization.yaml` de dev roto | `FALLA kustomize-build dev` y las 3 dependientes; prod sigue y da `OK`. `rc=1`. |
| PrometheusRule renombrada | `FALLA promtool-rules`. `rc=1`. |
| Red cortada | No la repetí: el código de ese camino no cambió en este diff. Sigue siendo `FALLA kubeconform` por `rc` distinto de 0 de la herramienta. |

F-01 queda cerrado. Las guardas atrapan por su propio mecanismo los tres casos de "cero trabajo". Los casos que ya cerraban bien siguen cerrando.

## 2. Fail-open nuevos que podrían haber introducido las guardas
- **Parseo de `Valid: N` y `N tests`.** Las guardas solo pueden subir `rc` a 1: son `[ ... ] || rc=1`, nunca lo bajan. Por eso un formato inesperado en la salida solo puede dar un falso `FALLA` (falla cerrado), nunca un `OK` indebido.
  - `n` sale de `sed -n` con `[0-9][0-9]*`, así que siempre es numérico. Si no hay coincidencia, `${n:-0}` da 0 y falla.
  - `Valid: ` con V mayúscula no confunde `Invalid: `.
  - `tail -1` toma el resumen, que es la última línea.
  - `\?` cubre el singular `1 test`.
- **Marca `.empty`.** Solo se crea cuando el build tuvo éxito y no hay ningún `^kind:`. Hace fallar kubeconform, conftest-test y conftest-combine de ese overlay. kustomize imprime `kind:` en la columna 0, y los contenidos multilínea van indentados, así que el `grep` no se confunde.
- **Afirmación del codificador sobre conftest con entrada vacía: verificada.** En el overlay vacío, la salida de la copia muestra `Summary: 0 resource found parsing stdin - Valid: 0...` seguido de `10 tests, 10 passed`. Conftest evalúa 10 tests sobre entrada vacía, así que la guarda `N tests` sola no habría bastado. La marca `.empty` es necesaria y funciona.
- **Overlay con documentos pero solo Namespaces** (copié `namespaces.yaml` dentro del overlay y lo usé como único recurso):
  - Ambos overlays: `OK kubeconform` y `OK conftest-test`, pero `FALLA conftest-combine` por la regla de conjunto de `default-deny`. `rc=1`.
  - Solo dev: `FALLA conftest-combine dev`. `rc=1`.
  - Aquí kubeconform y conftest-test dan `OK` legítimamente (51 y 510 tests en los otros casos, y recursos válidos). Quien lo atrapa es `default-deny`. Esa regla exige una NetworkPolicy específica, así que es una barrera real y no una casualidad. Un overlay que además trajera esa NetworkPolicy pasaría, lo cual es correcto.
- No encontré ningún fail-open nuevo.

## 3. F-03 (workflow)
- `timeout-minutes: 20` está en el job.
- `cancel-in-progress: ${{ github.event_name == 'pull_request' }}` está en el bloque `concurrency`.
- actionlint 1.7.7 no emite nada y da `rc=0`.
- Las convenciones de la ronda 1 siguen en pie: acción fijada por SHA completo, `permissions: contents: read` y triggers `push` y `pull_request`.
- `ci.yml` conserva `cancel-in-progress: true`. No fue tocado y eso queda fuera de alcance.

## Hallazgos
Ninguno ROJO ni NARANJA. F-01 y F-03 de la ronda 1 están corregidos. F-02 y la validación de `clusters/` quedaron como candidatas C-38 y C-39 por arbitraje del orquestador.

### AMARILLO (no bloquea)
- `bitacoras/U5-T15.md`, "Ronda 3": el CA-5 que debería ir en el informe de vuelta ya lo cubre este informe. No hace falta cambio.

## Tareas candidatas
- Ya registradas por el orquestador: C-38 (inventario mínimo de políticas y tests) y C-39 (validar `deploy/flux/clusters/{dev,prod}`).
- Nada nuevo.

## Rutas de transcripciones largas
- Salidas de stderr de mis copias, en `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/4ed52fd5-a142-46f0-ac9b-995972cfe97d/scratchpad/`: `ca1.txt`, `err-dev-resources-vacio.txt`, `err-dev-solo-namespaces.txt` y los demás `err-*.txt`.

```
VEREDICTO: VERDE
INFORME: revisiones/U5-T15/ronda-2.md
```
