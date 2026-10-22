# Ronda 1 — U10-T01

VEREDICTO: NO-VERDE (un NARANJA en pie, ningún ROJO)

Worktree `/home/omarjayg/Javeriana/topicos-especiales/agentic-qa-swarm-wt-U10-T01`, rama `tarea/U10-T01`, f2e4453 sobre 526eafe. Al terminar, `git status` salió limpio (`dist/` está ignorado y `.schema.check.ts` se borra solo). No escribí en el worktree. Mis scripts y mutaciones están en el scratchpad.

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | lint + typecheck + check:api + vitest | `npm ci` y luego el comando literal de la tarea | `0`. 17 archivos y 181 pruebas pasan, 9,8 s |
| 2 | Paridad de reglas | `npx vitest run src/styles/paridad 2>&1 \| grep -c failed` | `0` (3 pruebas pasan) |
| 3 | Tema + contraste AA | Suite completa (incluye `theme/tema.test.tsx` y `tokens.test.ts`) | pasa |
| 4 | CSP intacta | `npm run -s build >/dev/null && grep -c -E 'unsafe-inline\|unsafe-eval' dist/index.html` | build 0, conteo `0` |
| 5 | Alcance | El comando de la tarea (`git diff --name-only 526eafe HEAD \| grep -v -E …`) | `0` |

CA-5 pasa contra 526eafe, que es también `origin/main` y el merge-base. El `main` local está más adelante (8e6efe5). Por eso `git diff main...HEAD` lista ruido de U9-T06 y tareas. Es ruido del `main` local y no un desborde.

Comprobaciones adicionales mías:

**1. Paridad de verdad, con una mutación mía.** Mutilé en una copia del scratchpad 117 declaraciones de las 7 propiedades en reglas del inventario de `styles.css`. `paridad.test.ts` reportó exactamente 111 pares (selector, propiedad). Con un script postcss independiente calculé el conjunto esperado: todos los pares (selector, propiedad) de nivel superior del mockup, con el inventario y sin estado. Los dos conjuntos son idénticos, así que la prueba no filtra de más ni pasa por vacío. Hay 62 reglas del mockup con alguna de las 7 propiedades.

**2. Comparación completa con postcss.** Contra todas las reglas del inventario, incluidas `@media` y estados (96 reglas), con las 7 propiedades: 0 faltantes y 0 diferencias, incluyendo las `@media` que la prueba excluye. Comparando todas las propiedades solo difieren 3 valores:
- `.btn` transition: `0.12s` frente a `120ms`, equivalente.
- `.btn-primary` color: `#ffffff` frente a `var(--on-accent)`, mismo valor (`--on-accent` es `#ffffff` en ambos temas).
- `.btn:disabled` opacity: `0.5` frente a `0.55`, ver F-02.

La decisión de dejar `@media` fuera de la prueba es razonable. Aun así, hoy coinciden.

**3. Navegador headless** (Playwright 1.62 de `hermes-agent`, Chromium headless, `vite preview` con CSP estricta, API simulada con `page.route`):

| Viewport | Esquema | `.az-header-inner` | `<header>` | Desborde horizontal |
|---|---|---|---|---|
| 1280 | claro | 60 px | 61 px (60 + borde de 1 px; el mockup mide igual) | `scrollWidth` 1280 = `clientWidth`, sin desbordantes |
| 375 | claro | 154 px (la cabecera se envuelve, `flex-wrap` como en el mockup) | 155 px | `scrollWidth` 375 = `clientWidth`, sin desbordantes |
| 375 | oscuro | 154 px | 155 px | `scrollWidth` 375 = `clientWidth`, sin desbordantes |

- Chip: `OD | operador-demo | Rol: user`. Marca, navegación con iconos SVG, «Entorno», pestaña activa «Inbox» y botón «Salir» presentes. El screenshot `hdr-light.png` coincide con el del mockup (misma estructura y medidas).
- Tema: clic en el selector da `data-theme=dark`, `localStorage['aqs-tema']=dark` y fondo `rgb(15,20,31)`. Tras recargar, el tema sigue en `dark`. Con `colorScheme: dark` y sin valor guardado, arranca en `dark`.
- CSP: 0 mensajes de CSP en consola.
- Las vistas existentes conservan sus clases. Solo se eliminaron los `.app-*`, que usaba únicamente `ProtectedLayout`. Los demás selectores del CSS viejo siguen presentes y la suite completa pasa. `nav.test.tsx` solo cambió el nombre del enlace a «Entorno».

## Hallazgos
### F-01 · NARANJA · `web/dashboard/src/styles.css:469-476` y `src/components/Card.tsx` · `.card` arrastra relleno y margen que el mockup no tiene, y `Card pad={false}` no es «sin relleno»
- El `.card` del mockup no declara `padding` ni `margin-bottom`; su relleno es `.card-pad`. En el dashboard, `.card` lleva `padding: 16px; margin-bottom: 16px`, un resto heredado de las vistas actuales.
- Por eso `<Card pad={false}>` (cuyo comentario dice «sin relleno si pad es false») renderiza con 16 px de relleno y 16 px de margen inferior. `.kpi-box` ya tiene que anularlos con `margin-bottom: 0`.
- La prueba `componentes.test.tsx` solo comprueba `className === 'card'`, no el efecto, así que no lo ve.
- `paridad.test.ts` tampoco lo ve, porque solo exige lo que el mockup declara (un solo sentido). T02–T04 heredarán un `.card` que no es el del mockup y se desviarán sin que ninguna prueba falle. La tarea pide «solo las usen» con las mismas medidas.
- La bitácora lo reconoce como deuda diferida, pero el defecto vive en una primitiva entregada ahora. Arreglo posible: acotar el relleno y margen heredados a los selectores de las vistas actuales (por ejemplo `.page .card`, `.card.form`, `.card.detail`), o que `.card` coincida con el mockup y las vistas actuales reciban su relleno con una clase puente. Añadir además una prueba que compruebe el estilo computado o la ausencia de `padding` en `.card`.

### F-02 · AMARILLO · `styles.css` `.btn:disabled` · opacidad 0.55 frente a 0.5 del mockup
Desvío sin justificación. Está fuera de las 7 propiedades de CA-2, así que la paridad no lo detecta. Alinear a 0.5, o anotar el motivo (contraste) si es deliberado.

### F-03 · AMARILLO · `ProtectedLayout.tsx`, `SelectorTema.tsx` · cobertura de la cabecera y `aria-label` del tema
- Ninguna prueba cubre la estructura nueva del chip: iniciales, «Rol: …», «Salir» y `aria-label` del `nav`. Solo `nav.test.tsx` mira los 3 enlaces, y `LoginPage.test` toca `getByText('user')` de forma indirecta.
- El mockup actualiza el `aria-label` y el `title` del selector («Cambiar a tema claro» / «Cambiar a tema oscuro»). El dashboard los deja estáticos. No lo exige un CA.

### F-04 · AMARILLO · `tokens.css:45` · pila tipográfica distinta a la del mockup
El mockup usa `"IBM Plex Sans"` con respaldo. El dashboard usa solo la pila del sistema (decisión previa de U9-T04, con comentario). En el screenshot el logo «aqs» y el texto se ven con otra tipografía. No es de esta tarea, pero afecta a la meta «se ven muy distintos» de U10. Es para T05 o para una candidata.

### F-05 · AMARILLO · `tema.ts` · el tema no sigue al sistema en caliente
Si no hay valor guardado y el sistema cambia mientras la página está abierta, no se refleja hasta recargar. La bitácora lo documenta. Aceptable.

Notas sin hallazgo:
- La evidencia de «rojo primero» (53 «falta la regla») y la mutación de control de `.btn-sm` están en la bitácora, y mi mutación masiva las confirma.
- `Kpi` usa `style={{width}}`. React lo aplica por CSSOM, que `style-src 'self'` permite. No vi violaciones de CSP, pero `Kpi` aún no se renderiza en ninguna vista, así que no está ejercitado en navegador.
- `.az-container` está definida pero sin usar, tal como indica el alcance.

## Tareas candidatas (fuera de alcance)
- `DESIGN.md` (§1, §4.1, §5.x) sigue nombrando «Warm» como etiqueta de navegación (ya la anotó el codificador).
- Vendorizar IBM Plex Sans o Mono con `font-src 'self'` para acercar la tipografía al mockup (F-04), idealmente en T05.
- Para T05: que la paridad sea bidireccional, comprobando que las reglas del dashboard no añadan propiedades de las 7 que el mockup no declare, y que cubra `@media` y estados, que hoy sí coinciden.

## Rutas de transcripciones y artefactos (scratchpad)
- `/tmp/claude-1000/-home-omarjayg-Javeriana-topicos-especiales-agentic-qa-swarm/2b25eeb9-6a55-4813-b08e-b008f856522d/scratchpad/pw.cjs` — script Playwright.
- `.../scratchpad/hdr-light.png`, `hdr-dark.png`, `hdr-375.png` — cabecera del dashboard en claro, oscuro y 375 px.
- `.../scratchpad/mk-hdr.png` — cabecera del mockup.
- `.../scratchpad/cmp.cjs` y `cmp2.cjs` — comparación postcss.
- `.../scratchpad/mut.cjs` — mutación masiva de la paridad.

VEREDICTO: NO-VERDE
NARANJA|web/dashboard/src/styles.css:469 y src/components/Card.tsx|`.card` con padding/margin-bottom heredados distintos del mockup; Card pad={false} no es «sin relleno» y ninguna prueba lo detecta
