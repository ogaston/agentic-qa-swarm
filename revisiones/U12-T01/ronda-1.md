# Ronda 1 — U12-T01

VEREDICTO: VERDE

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Calidad intacta | `npm run -s lint && typecheck && check:api && npx vitest run \| grep -c …` | pasa (`0`). Suite completa: 23 archivos y 273 pruebas, todas pasan, 16 s. |
| 2 | Paso 2 sin resumen y con tres familias | `npx vitest run src/pages/Inbox -t 'familias'` | pasa. La prueba nueva está entre las 3 que coinciden, con 0 fallos. |
| 3 | La confirmación envía solo `["happy-path"]` | `npx vitest run src/pages/Inbox -t 'envía'` | pasa. La prueba lee el cuerpo del POST capturado por MSW y comprueba `{flows:['happy-path']}`. |
| 4 | Sin `style={{` nuevos (reformulado por el orquestador) | `git diff main..HEAD -- web \| grep -c '^+.*style={{'` | `0` líneas añadidas en código. El `grep -rn` global da 3, todos preexistentes en `Kpi.tsx` y `LandingPage.tsx`, y C-135 los registra. |
| 5 | Alcance | comando del CA-5 | `0` archivos fuera de la lista permitida. |

El worktree quedó limpio después de la revisión (`git status --short` sin salida).

## Hallazgos
Ninguno ROJO ni NARANJA.

### F-01 · AMARILLO · `InboxPage.test.tsx` · La prueba de CA-2 no comprueba el orden de las familias ni el `aria-label`
Está bien como está. Un caso de borde secundario sería comprobar que hacer clic en un checkbox deshabilitado no cambia `familias`. El `onChange` ya lo protege con `esDisponible`.

### F-02 · AMARILLO · `InboxPage.tsx` · `aria-label` duplica la etiqueta visible
`aria-label={familia.etiqueta}` coincide con el texto del `<span>`. Evita que el nombre accesible incluya «Próximamente», que es lo que necesita el CA-2. Es una decisión consciente y no bloquea.

## Notas de verificación
- **Alcance:** el diff toca solo `InboxPage.tsx`, `InboxPage.test.tsx`, `flows.ts`, `styles.css` y la bitácora.
- **Tipos y cuerpo de `POST /confirm`:** `FlowFamily` se deriva de las familias con `disponible: true` (hoy solo `happy-path`). El cuerpo de `POST /confirm` no cambia.
- **Estado inicial y reinicio:** ambos usan `familiasDisponibles()`.
- **Insignia:** «Próximamente» es texto, no solo color.
- **Valores incrustados:** las etiquetas viven en `flows.ts`, no en un punto de decisión.
- **Rojo primero:** la bitácora pega el fallo de CA-2 antes del cambio.
- **Pruebas existentes modificadas (foco atrapado y enlaces del cambio):** ambas se ajustaron por la eliminación legítima del resumen. Ninguna se omitió ni se debilitó sin motivo.
- **CSS:** las clases nuevas usan tokens existentes (`--text-muted`, `--surface-2`, `--border-subtle`).

## Tareas candidatas
- C-135, ya registrada: limpiar los `style={{` de `Kpi.tsx` y `LandingPage.tsx`.

## Rutas de transcripciones largas
- Ninguna.

```
VEREDICTO: VERDE
INFORME: revisiones/U12-T01/ronda-1.md
```
