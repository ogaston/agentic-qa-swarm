# U12-T01 — Paso 2 del modal: solo familias de flujos, con familias futuras deshabilitadas

**Unidad:** U12 — Control humano de la ejecución
**Historias que implementa:** US-M2 (confirmación humana), S5 (dashboard).
**Depende de:** nada. **Ola 1**. **Tope: 3 rondas**; si la ronda 3 sale NO-VERDE se escala al humano.

---

## Alcance

**Dentro** (una línea):

> El paso 2 del modal de Inbox («Confirmar corrida») muestra solo el selector de familias de flujos: `happy-path` elegible y marcada por defecto, y «Pruebas de carga» y «Pruebas de seguridad» visibles pero deshabilitadas con la marca «Próximamente».

Detalle:

- En `web/dashboard/src/pages/Inbox/InboxPage.tsx`, el paso 2 (rama `panelAbierto`) deja de mostrar la sección «Resumen del cambio» (`aria-labelledby="modal-resumen-titulo"`). El paso 1 («Detalles del cambio») no cambia.
- `web/dashboard/src/inbox/flows.ts` pasa a describir cada familia como objeto: `{ id, etiqueta, disponible }`. Familias: `happy-path` (etiqueta `happy-path`, disponible), `load` («Pruebas de carga», no disponible), `security` («Pruebas de seguridad», no disponible). `FlowFamily` sigue siendo el tipo de los ids **disponibles** (hoy solo `'happy-path'`), de modo que el cuerpo de `POST /confirm` no cambia.
- Las familias no disponibles se pintan con `<input type="checkbox" disabled>` sin marcar y una insignia de texto «Próximamente» (el estado no se comunica solo con color). Nunca se envían en la confirmación.
- El estado inicial y el reinicio de `familias` marcan solo las familias disponibles.
- Estilos: reutilizar clases y tokens existentes de `src/styles.css`; si hace falta una clase para la opción deshabilitada, se añade allí. Sin `style={{}}`.

**Fuera** (rechazar aunque se vea roto):

- La paginación del Inbox y cualquier otra parte de la tabla.
- El paso 1 del modal, el texto del pie y los botones.
- Contratos OpenAPI y backend: las familias nuevas no existen en el servidor.
- Implementar pruebas de carga o seguridad.

---

## Archivos de contexto

- `web/dashboard/src/pages/Inbox/InboxPage.tsx` (rama `panelAbierto`, `familias`, `alternarFamilia`, `confirmar`)
- `web/dashboard/src/pages/Inbox/InboxPage.test.tsx`
- `web/dashboard/src/inbox/flows.ts`
- `web/dashboard/src/styles.css` (`.modal-flow-option`, `.modal-flows`)
- `DESIGN.md` (§6.2: sin estilos en línea)

---

## Criterios de aceptación

Desde `web/dashboard/`, con `npm ci` hecho.

- [ ] **CA-1** — Calidad intacta.
  ```bash
  npm run -s lint && npm run -s typecheck && npm run -s check:api && npx vitest run 2>&1 | grep -c -E '✗|×| failed|FAIL'
  ```
  Esperado: `0`.

- [ ] **CA-2** — El paso 2 no muestra el resumen y sí las tres familias. Prueba nueva en `InboxPage.test.tsx` que abre el paso 2 y comprueba: no existe el encabezado «Resumen del cambio»; existe el checkbox `happy-path` marcado y habilitado; existen los checkboxes «Pruebas de carga» y «Pruebas de seguridad» `disabled` y sin marcar; aparece dos veces el texto «Próximamente».
  ```bash
  npx vitest run src/pages/Inbox -t 'familias' 2>&1 | tail -n 6
  ```
  Esperado: la prueba nueva aparece y pasa; 0 fallos.

- [ ] **CA-3** — La confirmación solo envía familias disponibles. Prueba que confirma y lee de vuelta el cuerpo de la petición `POST` capturada por el doble de `fetch`: `flows` es exactamente `["happy-path"]`.
  ```bash
  npx vitest run src/pages/Inbox -t 'envía' 2>&1 | tail -n 6
  ```
  Esperado: pasa.

- [ ] **CA-4** — Sin estilos en línea.
  ```bash
  grep -rn 'style={{' src | wc -l
  ```
  Esperado: `0`.

- [ ] **CA-5** — Alcance.
  ```bash
  b=$(git merge-base HEAD main); git diff --name-only $b | grep -v -E '^(web/dashboard/src/(pages/Inbox/InboxPage(\.test)?\.tsx|inbox/flows\.ts|styles\.css)|bitacoras/U12-T01(\.md|/)|revisiones/U12-T01/)' | wc -l
  ```
  Esperado: `0`.

---

## Plan de pruebas

- **Rojo primero:** escribir CA-2 antes de tocar el componente y pegar su fallo en la bitácora.
- CA-3 lee el cuerpo real enviado, no un estado interno.
