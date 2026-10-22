# U9-T06 — Limpieza de estilo del dashboard (C-130, C-131)

**Unidad:** U9 — Resultados visibles en el dashboard
**Historias que implementa:** S5 (dashboard presentable).
**Depende de:** U9-T04 **fusionada** (ya lo está). **Ola 4**. **Tope: 3 rondas**; si la ronda 3 sale NO-VERDE se escala al humano.

---

## Alcance

**Dentro** (una línea, concreta):

> Quitar el único `style={{}}` del dashboard (C-131) y dar al dashboard una comprobación de URLs externas en `dist` que no dé falsos positivos (C-130), sin cambiar comportamiento.

Detalle:

- **C-131.** En `web/dashboard/src/pages/Run/RunPage.tsx`, el aviso de fase fallida deja de usar `style={{ color, fontWeight }}` y la constante `COLOR_FALLIDA`; el color y el peso ya los da la clase `.status-failed` (`src/styles.css`, tokens `--failed-*`). `RunPage.test.tsx` deja de exigir `rgb(176, 0, 32)` (jsdom no carga la hoja de estilos) y comprueba `data-estado="failed"` y la clase `status-failed`. Nada más cambia en esa prueba.
- **C-130.** Un script `web/dashboard/scripts/check-dist-urls.sh`, expuesto como `npm run check:dist-urls`, que sobre `dist/` cuenta URLs absolutas `http(s)://` **solo** en contextos que cargan recurso: atributos `src`/`href`, `url(...)` y `@import`. Excluye `w3.org` (espacios de nombres SVG). Ignora las bases de parseo del código (`src/api/client.ts` `http://localhost/`, `src/session/next.ts` `https://destino.invalid`) y el enlace del decodificador de errores de React, porque no cargan nada. Imprime el conteo y sale con código distinto de cero si es mayor que 0.

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- C-132 (fuentes IBM Plex): decisión del humano, pendiente. No se toca la tipografía.
- Cambios de aspecto, tokens o estructura de las vistas.
- Cambios en la CSP, en `dist/index.html` o en otras pruebas.
- Cambios fuera de `web/dashboard/**` salvo los archivos de esta tarea.

---

## Archivos de contexto

- `web/dashboard/src/pages/Run/RunPage.tsx`, `RunPage.test.tsx`
- `web/dashboard/src/styles.css` (`.status-failed`)
- `DESIGN.md` §6.2 (prohíbe `style={{}}`)
- `tareas/U9-T04-estilo-del-dashboard.md` (CA-2, el filtro que se corrige)

---

## Criterios de aceptación

Desde `web/dashboard/`, con `npm ci` hecho.

- [ ] **CA-1** — Comportamiento intacto.
  ```bash
  npm run -s lint && npm run -s typecheck && npm run -s check:api && npx vitest run 2>&1 | grep -c -E 'failed|FAIL'
  ```
  Esperado: `0`. `git diff --stat origin/main -- 'src/**/*.test.tsx'` solo toca `RunPage.test.tsx`.

- [ ] **CA-2** — Sin estilos en línea.
  ```bash
  grep -rn 'style={{' src | wc -l
  ```
  Esperado: `0`.

- [ ] **CA-3** — El fallo sigue distinguible sin color solo.
  ```bash
  npx vitest run src/pages/Run 2>&1 | tail -n 4
  ```
  Esperado: sin `failed`; la prueba «failed es terminal» comprueba `data-estado="failed"`, la clase `status-failed` y el texto «Fallida».

- [ ] **CA-4** — Comprobación de URLs, sin falsos positivos y con sensibilidad.
  ```bash
  npm run -s build >/dev/null && npm run -s check:dist-urls; echo "salida=$?"
  mkdir -p /tmp/aqs-urls && printf '<link href="https://cdn.example.com/x.css">' > dist/_prueba.html && npm run -s check:dist-urls; echo "salida=$?"; rm -f dist/_prueba.html
  ```
  Esperado: primera ejecución imprime `0` y `salida=0`; la segunda imprime `1` y `salida=1` (la prueba de sensibilidad).

- [ ] **CA-5** — Alcance.
  ```bash
  cd ../.. && git status --short | wc -l; b=$(git merge-base HEAD origin/main); git diff --name-only $b | grep -v -E '^(web/dashboard/(src/pages/Run/|scripts/check-dist-urls\.sh|package\.json)|tareas/U9-T06-limpieza-estilo\.md|bitacoras/U9-T06(\.md|/)|revisiones/U9-T06/)' | wc -l
  ```
  Esperado: `0` y `0`.

---

## Plan de pruebas

- **Rojo primero:** pegar la salida de `grep -rn 'style={{' src | wc -l` (da `1`) y de la prueba actual antes de implementar.
- La sensibilidad de CA-4 demuestra que el script detecta una URL que sí carga un recurso.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
