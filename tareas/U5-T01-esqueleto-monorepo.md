# U5-T01 — Esqueleto del monorepo

**Unidad:** U5 — Plataforma & GitOps
**Historias que implementa:** US-M10
**Depende de:** ninguna (es la primera tarea de la secuencia U5 → U1/U4 → U2 → U3)

---

## Alcance

**Dentro** (una línea, concreta):

> Crear el árbol de directorios versionable del monorepo (`services/`, `agents/`, `contracts/openapi/`, `contracts/events/`, `deploy/flux/{base,dev,prod}/`, `.github/workflows/`) con un marcador `.gitkeep` en cada directorio que quedaría vacío, más un `README.md` de layout que nombre cada directorio, y dejarlo en un commit inicial con el árbol limpio.

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Definir el contenido de los contratos (OpenAPI y esquemas de eventos): es **U5-T02**.
- Escribir workflows de GitHub Actions: es **U5-T03**.
- Escribir manifiestos Flux/Kustomize/Helm (incluido el entorno warm): es **U5-T04**.
- Implementar cualquier servicio (`services/**`) o agente (`agents/**`): ninguna tarea de U5 los implementa.
- Añadir licencias, linters o configuraciones de toolchain no pedidas en esta tarea.
- Cualquier `kubectl`, `helm`, `terraform` o `apply` contra un clúster (esta tarea no toca infraestructura).

---

## Archivos de contexto

Rutas, no contenido pegado:

- `unidades-y-tareas.md`
- `aidlc-docs/inception/application-design/unit-of-work.md` (layout del monorepo)
- `aidlc-docs/inception/application-design/unit-task-plans/U5.md`
- `aidlc-docs/inception/application-design/unit-of-work-story-map.md`
- `aidlc-docs/inception/application-design/unit-of-work-dependency.md`

---

## Criterios de aceptación

Cada uno con **su comando**. El revisor los va a correr él mismo, uno por uno.

- [ ] **CA-1** — Existe el árbol de directorios del monorepo.
  ```bash
  for d in services agents contracts/openapi contracts/events deploy/flux/base deploy/flux/dev deploy/flux/prod .github/workflows; do test -d "$d" || { echo "FALTA $d"; exit 1; }; done; echo "ARBOL OK"
  ```
  Esperado: `ARBOL OK`. Antes de la tarea: `FALTA services` (este es el rojo inicial).

- [ ] **CA-2** — Los directorios que quedarían vacíos son versionables (trackeados por git).
  ```bash
  git ls-files -- services agents contracts deploy .github | grep -c -E '(^|/)\.gitkeep$'
  ```
  Esperado: `8` (un `.gitkeep` por cada directorio del árbol que no tiene otro contenido). Acotado a las rutas del monorepo para no contar el andamiaje del loop (`bitacoras/`, `revisiones/`); enmienda aprobada por el humano tras `revisiones/U5-T01/ronda-1.md` (F-01).

- [ ] **CA-3** — El `README.md` de layout nombra cada directorio raíz.
  ```bash
  grep -E 'services/|agents/|contracts/|deploy/flux/|\.github/workflows/' README.md | wc -l
  ```
  Esperado: un número ≥ 5.

- [ ] **CA-4** — El árbol de trabajo queda limpio tras el commit.
  ```bash
  git status --short
  ```
  Esperado: sin líneas.

---

## Plan de pruebas

Qué pruebas se escriben **en esta tarea**, no después:

- Ejecución de CA-1 **antes** de crear el árbol (rojo) y **después** (verde). El par falla-pasa es la prueba.
- Comprobación de que los `.gitkeep` están realmente trackeados y no ignorados: `git check-ignore services/.gitkeep .github/workflows/.gitkeep` (esperado: sin salida) y `git ls-files --error-unmatch services/.gitkeep` (esperado: ruta impresa).
- Comprobación negativa: si se borra un directorio y su `.gitkeep`, CA-1 vuelve a fallar (verifica que CA-1 no pasa por casualidad).

**Rojo primero:** el codificador reproduce `FALTA services` y lo registra en su bitácora antes de crear nada.

---

## Notas

- Archivos que se **modifican en su sitio**: `README.md` (crear una sola vez; si ya existe, se amplía, nunca se duplica). Nada de `README-nuevo.md` ni variantes con sufijo.
- Un `.gitkeep` **solo** en directorios que de otro modo quedarían vacíos. No añadir `.gitkeep` a directorios que ya contienen un archivo real.
- El layout canónico está en `unit-of-work.md`: no inventar directorios adicionales ni renombrar los existentes.
- Esta tarea no ejecuta ningún comando contra un clúster ni la nube.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
