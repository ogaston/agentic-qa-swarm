# Ronda 1 — U5-T15

VEREDICTO: NO-VERDE

Los 5 criterios de aceptación pasan con mis propias ejecuciones, pero el script falla abierto en tres casos de "cero comprobaciones". Son un NARANJA.

Hash de la tarea: 5cec844b357aafbc30689deb0cd7f546041d8de8 (coincide). HEAD del worktree: d8789ce.

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | El script existe y pasa | `test -x scripts/ci/policies.sh && bash scripts/ci/policies.sh >/dev/null 2>&1; echo rc=$?` | pasa: `rc=0` (12 s) |
| 2 | 5 comprobaciones en ambos overlays | el `grep -c -E` literal | pasa: `8`. Las 8 líneas son `OK`. Los totales de herramienta no son cero: 51 recursos válidos, 510 y 10 tests de conftest por overlay, 66 tests de `conftest verify`. |
| 3 | 4 negativas | `neg()` literal ×4 | pasa: politica `rc=1`, default-deny `rc=1`, kubeconform `rc=1`, promtool `rc=1` |
| 4 | Workflow | actionlint 1.7.7 y los 4 `grep` | pasa: `rc=0`, luego `2`, `1`, `0`, `1` |
| 5 | shellcheck, alcance, árbol limpio | shellcheck v0.10.0, `git diff --name-only`, `git status --short \| wc -l` | pasa: `rc=0`; exactamente los 3 archivos esperados; `0` |

El worktree quedó limpio después de la revisión. Mis copias temporales están en el scratchpad de la sesión.

## Fail-open: casos provocados en copias de `git archive HEAD`
Los dos primeros "rc" de la lista son del script; el tercero resume los casos de fallo cerrado.

| Caso provocado | Resultado |
|---|---|
| Se borra `policy/` | `FALLA` en conftest-test ×2, conftest-combine ×2 y conftest-verify; `rc=1`. Cierra bien. |
| `policy/` sin ningún `.rego` | `FALLA` en conftest-test, conftest-combine y conftest-verify; `rc=1`. Cierra bien. |
| Catálogo CRD devuelve 404 | `FALLA kubeconform` dev y prod (7 errores de "could not find schema"); `rc=1`. Cierra bien. |
| Red cortada para kubeconform (`--network none`) | `FALLA kubeconform` dev y prod ("failed downloading schema"); `rc=1`. Cierra bien. |
| `kustomization.yaml` de dev roto | `FALLA kustomize-build dev` y las 3 comprobaciones dependientes; prod sigue y se ejecuta. Cierra bien. |
| Se renombra la PrometheusRule | `FALLA promtool-rules`, porque la extracción queda vacía y `[ -s ]` lo detecta. Cierra bien. |
| Se borra `rules_test.yaml` | `FALLA promtool-rules`. Cierra bien. |
| `rules_test.yaml` vacío | `FALLA promtool-rules`. Cierra bien. |
| **`rules_test.yaml` con `tests: []`** | **`OK promtool-rules`, `rc=0`. Falla abierto: promtool no ejecutó ningún test.** |
| **Overlay con `resources: []`** | **kubeconform da `OK` con "0 resource found" y conftest-test da `OK` sobre entrada vacía. Falla abierto.** Solo `conftest-combine` lo atrapa, por casualidad: `default-deny` exige NetworkPolicies. |
| **Se borran `policy/security.rego` y `security_test.rego`** | **`rc=0`: ocho `OK`.** Una política desaparece sin que nada lo note (el total baja a 204 y 38 tests, pero nadie lo mira). |

No hay `|| true` en el script. No hay pipes, así que `pipefail` no tiene efecto, y la falta de `set -e` es deliberada para ejecutarlo todo. El único `||` es `cd "$root" || exit 1`.

## Hallazgos
### F-01 · NARANJA · `scripts/ci/policies.sh:58-81` · Sin guarda de "cero comprobaciones" (clase: un `OK` con 0 trabajo hecho)
Tres instancias de la misma clase, todas demostradas arriba:
1. **kubeconform** con 0 recursos reporta `OK`. Solo mira el código de salida.
2. **conftest-test** con entrada vacía reporta `OK`.
3. **promtool** con `tests: []` reporta `OK`. Es lo que ocurre si un PR vacía la lista de tests.

El script ya aplica esta guarda para las reglas extraídas (`[ -s rules.yaml ]`), pero no la aplica en los otros tres sitios. Hoy `conftest-combine` tapa el caso del overlay vacío por casualidad. Esa casualidad no es el diseño.

Qué pedir:
- Un `[ -s "$work/$e.yaml" ]` tras el build.
- Comprobar que el resumen de kubeconform tenga `Valid: N` con N>0, o contar los documentos con yq.
- Comprobar que `rules_test.yaml` tenga al menos un test, por ejemplo con `yq '.tests | length'` mayor que 0.

El CA-3 no cubre estos casos. Una negativa por cada guarda lo cerraría.

### F-02 · AMARILLO · `scripts/ci/policies.sh:60-67` · Sin inventario mínimo de políticas
Borrar una política junto con su test deja `rc=0` (ejemplo de `security.rego` arriba). No hay nada que fije "deben existir estas N políticas". Los 510 tests de conftest-test bajan a 204 sin que nadie lo vea. No bloquea: es un problema de revisión del PR. Lo anoto como mejora.

### F-03 · AMARILLO · `.github/workflows/policies.yml` · Detalles de convención
- No hay `timeout-minutes`. Con el repo en un runner que baja cuatro imágenes, un cuelgue de red consumiría 6 h.
- `cancel-in-progress: true` también se aplica a `push` en `main`. Un push posterior puede cancelar la corrida de un commit anterior de `main`. `ci.yml` usa el mismo patrón (`ci-${{ github.ref }}`), así que es coherente con el repo. Aun así, para un guardián único conviene `cancel-in-progress: ${{ github.event_name == 'pull_request' }}`.

### Workflow, punto por punto
- La acción está fijada por SHA completo: `actions/checkout@11d5960a326750d5838078e36cf38b85af677262` (v4.4.0). `git ls-remote https://github.com/actions/checkout 'refs/tags/v4.4.0*'` devuelve ese mismo SHA.
- `permissions: contents: read` está a nivel de archivo y no hay más permisos.
- Los triggers son `push` y `pull_request`.
- `continue-on-error` aparece 0 veces.
- El paso es `run: bash scripts/ci/policies.sh` sin `|| true` ni redirección. Si el script sale distinto de 0, el job falla.
- `runs-on: ubuntu-24.04`, igual que `ci.yml`. `contracts.yml` usa `ubuntu-latest`; no es un defecto de esta tarea.
- Las imágenes están fijadas por tag. `mikefarah/yq:4.44.3` no figura en la lista de la tarea, pero sí en las tareas U5-T05, U5-T06 y U5-T13, así que es coherente con U5.

### AVISO sobre `deploy/flux/clusters/`
Este punto es correcto. `deploy/flux/clusters/` existe ahora (U5-T13 está fusionado, con `dev/` y `prod/`). El script solo emite `AVISO ... existe pero no se valida` por stderr, con `rc=0`, y no omite ninguna otra comprobación: el AVISO no tapa errores. Un `kustomize build deploy/flux/clusters/dev` funciona (12540 líneas), así que validarlo es viable.

## Tareas candidatas
- Validar `deploy/flux/clusters/{dev,prod}` en `policies.sh`. U5-T13 ya está fusionado y el AVISO queda como ruido permanente.
- Inventario mínimo de políticas y de tests: contar los `.rego` y los tests, y fallar si bajan sin un cambio explícito (F-02).
- `timeout-minutes` y `cancel-in-progress` condicional en los workflows (F-03).

## Rutas de transcripciones largas
- Salida completa de la corrida de CA-1/CA-2: `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/4ed52fd5-a142-46f0-ac9b-995972cfe97d/scratchpad/out.txt`
- Copias negativas `neg1` a `neg6`, con su `err`, en el mismo directorio de scratchpad.

```
VEREDICTO: NO-VERDE
NARANJA|scripts/ci/policies.sh:58-81|Sin guarda de cero comprobaciones: kubeconform con 0 recursos, conftest-test con entrada vacía y promtool con tests: [] dan OK con rc=0
INFORME: revisiones/U5-T15/ronda-1.md
```
