# Ronda 2 — U5-T04

VEREDICTO: VERDE

Corrí CA-1..CA-8 y las dos pruebas negativas sobre copias temporales de HEAD `028c8b10be2233e879c6bcb98534a3e7625ed94f`. Todo pasa y ningún hallazgo bloqueante sigue en pie. La tarea tiene el hash esperado (9d9207a...).

## Criterios de aceptación, verificados por mí
Comandos literales de la tarea, con `K` como en la tarea. CA-1..CA-7 los corrí sobre `git archive HEAD` en el scratchpad. CA-8 lo corrí en el worktree.

| # | Criterio | Resultado |
|---|---|---|
| 1 | Los dos overlays construyen | `BUILD OK` |
| 2 | kubeconform estricto con CRDs de Flux | dos líneas `26 resources found parsing stdin - Valid: 26, Invalid: 0, Errors: 0, Skipped: 0` |
| 3 | Warm completo en `aqs-test` | `6` y `6` |
| 4 | 7 servicios en `aqs-system` | `14` |
| 5 | Imágenes de prod sin `latest` ni sin tag | `0` y `0` |
| 6 | Kustomization de Flux con path propio y prune | `2` y `2` |
| 7 | Contenedores, limits y probes | `12`, `12`, `6` |
| 8 | Árbol limpio y sin `.gitkeep` | `0` y `0` |

Con un grep tolerante (`^\s*(- )?image:`) el conteo de imágenes también da 12.

**Pruebas negativas, sobre copia temporal:**
- **CA-5, `go-intake:latest`:** el primer conteo dio `0` y el segundo `1`, así que detecta.
- **CA-2, `replicaz`:** salió `additionalProperties 'replicaz' not allowed` y `Valid: 25, Invalid: 1`.

**Evidencia vs mi ejecución:** la evidencia fresca de la bitácora (ronda 2) coincide con la mía. Las cifras sí pudieron salir del diff actual, porque el HEAD solo añade comentarios y `PGDATA`, y no cambia nada que altere los conteos.

**Árbol y alcance:**
- `git status --short` del worktree quedó vacío antes y después. No dejé cambios.
- `git diff --name-only 8819ac5..tarea/U5-T04` toca solo `deploy/flux/**` y `bitacoras/U5-T04.md`. No hay desborde de alcance nuevo.
- Lo nuevo de esta ronda (`fa18a7d..HEAD`) son 83 líneas en 3 archivos: comentarios YAML, una variable `PGDATA` y la bitácora.

## Verificación de los hallazgos de la ronda 1

### F-01 · NARANJA · resuelto
La sección "Ronda 2" de la bitácora pega cada CA con su comando literal y su salida. El rojo inicial usa el `exit 1` literal, reproducido sobre la base 8819ac5. Las dos negativas traen el `sed` completo. La salida de CA-1..CA-7 coincide con la mía.
- **CA-8 final:** la bitácora lo remite al informe de vuelta y anula la nota huérfana "ver CA-8 final al pie" de la ronda 1. No puede vivir en un archivo ya commiteado, y mi propia ejecución lo da en `0` y `0`.
- **Secciones de la ronda 1:** siguen con la forma abreviada `$ CA-n`. Quedan sustituidas por la ronda 2 y no se reescribieron, como pidió el arbitraje.

### F-02 · NARANJA · resuelto
La bitácora añade la tabla "Decisiones no fijadas por la tarea", con valor, razón y quién confirma. Cubre todo lo que señalé:
- `flux-system` y el `sourceRef` GitRepository `agentic-qa-swarm`, marcado como supuesto del bootstrap;
- los placeholders `nginx:1.27.2` y `go-reset:0.0.0`, con los argumentos inventados `housekeeping` y `rebuild`;
- `idleScaleDownAfter: 30m` y `minReplicasIdle: "0"`;
- los schedules `0 * * * *` y `0 3 * * 0`;
- el Secret `warm-db-credentials` (clave `password`), referenciado y no creado.

### F-03 · AMARILLO, especificación · conforme al arbitraje
Los 10 contenedores con `env: TZ=UTC` llevan ahora un comentario YAML que explica la razón. Lo conté en `warm.yaml` y `control-plane.yaml`, y el diff de la ronda los muestra. El contenedor `postgres` no lo lleva, porque `PGDATA` ya cumple esa función. La enmienda de los greps de CA-5 y CA-7 sigue pendiente del humano. No lo cuento como defecto del codificador.

### F-04 · AMARILLO · documentado
La bitácora declara que los patches de dev igualan a base y que la diferencia real dev/prod está solo en prod. Verificado, y consistente con lo que reporté. Sin cambio de alcance.

### F-05 · AMARILLO · documentado
La bitácora anota que el mensaje de `fbe9222` es inexacto y que no se reescribe el historial. Así lo pidió el arbitraje.

### F-06 · AMARILLO · corregido
`warm-db` fija `PGDATA=/var/lib/postgresql/data/pgdata`, mientras el `volumeMount` sigue en `/var/lib/postgresql/data`. El subdirectorio queda dentro del PVC, así que `lost+found` ya no estorba al `initdb`. El cambio no rompió ningún criterio, y la variable `env` además mantiene a `image` lejos de ser la primera clave en ese contenedor, así que CA-7 sigue contando 12.

## Hallazgos nuevos
Ninguno ROJO ni NARANJA. Un matiz AMARILLO menor:
- **`PGDATA` y el grep de CA-7:** el contenedor `postgres` depende de `PGDATA` (o de otro `env`) para que el grep de CA-7 lo cuente. Esa dependencia no está comentada. No bloquea, y la enmienda de especificación pendiente en F-03 la elimina.

## Tareas candidatas
- Enmendar CA-5 y CA-7 de la tarea a `^\s*(-\s+)?image:` (el humano ya lo tiene pendiente).
- Definir el GitRepository y el namespace `flux-system` de bootstrap que consumen los Kustomizations.
- Decidir quién crea `warm-db-credentials`. Si hay securityContext para el warm, va en U5-T05..T08.

## Rutas de transcripciones largas
Ninguna. Las copias temporales están en `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/4ed52fd5-a142-46f0-ac9b-995972cfe97d/scratchpad/r2`.

El informe no lo escribí a `revisiones/U5-T04/ronda-2.md`, porque no tengo herramienta de escritura. El orquestador debe guardar este texto en esa ruta.

VEREDICTO: VERDE
AMARILLO|criterio CA-7 / deploy/flux/base/warm.yaml (postgres)|El conteo de CA-7 depende de PGDATA/env para el grep, sin comentario (no bloquea)
INFORME: revisiones/U5-T04/ronda-2.md
