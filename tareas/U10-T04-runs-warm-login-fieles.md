# U10-T04 — «Mis corridas», Entorno y Login fieles al mockup

**Unidad:** U10 — Fidelidad visual del dashboard al mockup
**Historias que implementa:** S5, US-M10 (estado del warm), US-M1.
**Depende de:** U10-T01 **fusionada**. **Ola 2.** **Tope: 3 rondas**.

## Alcance
**Dentro:**
> Rehacer `/runs`, `/warm` y `/login` según las vistas «Mis corridas» (~1446), «Entorno» (~1490) y «Iniciar sesión» (~1552) del mockup.

- **Mis corridas:** `PageHeader` + tarjeta con tabla (run_id enlazado, notification_id, confirmado el, confirmado por), vacío con mensaje.
- **Entorno:** `PageHeader`, tarjeta «Estado operativo de <warm_id>» con `kv-list` (estado con insignia + icono, `reset_verified`, `baseline_version`, `warm_id`), alerta de reset no verificado (`alert-warn`/`alert-danger`) y estados `ready/dirty/cuarentena/idle-escalado`.
- **Login:** `login-wrap` centrado, tarjeta con marca, `form-field` con etiquetas visibles, TOTP opcional, errores 401/429 (cuenta atrás)/503 con `alert-box`, foco visible; sin cabecera de sesión.
**Fuera:** endpoints nuevos; mostrar datos que la API no entrega; cambiar el flujo de autenticación.

## Archivos de contexto
`mockups/swarm-mock.html`, `DESIGN.md`, `src/pages/{Runs,Warm,Login}/*`.

## Criterios de aceptación
- [ ] **CA-1** — `npm run -s lint && npm run -s typecheck && npx vitest run 2>&1 | grep -c -E 'failed|FAIL'` → `0`; el diff de pruebas solo cambia selectores.
- [ ] **CA-2** — Pruebas de estructura para las tres vistas (rol/nombre) y para los 4 estados del warm con texto además de color.
- [ ] **CA-3** — 375 px sin scroll horizontal (prueba en el e2e de T05 o captura en la bitácora).
- [ ] **CA-4** — Fidelidad: `npm run -s e2e:fidelidad -- runs warm login` (U10-T05) o capturas lado a lado.
- [ ] **CA-5** — CSP y alcance como en T02.

_Creado con ❤️ por Luis Felipe Ariza Vesga._
