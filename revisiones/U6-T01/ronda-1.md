# Ronda 1 — U6-T01

VEREDICTO: VERDE

Worktree `/home/omarjayg/Javeriana/topicos-especiales/agentic-qa-swarm-wt-U6-T01-web`, rama `tarea/U6-T01-web`, sha `facf0c9`. El hash de la tarea (`git hash-object`) es `7b780464bfac9cbaad53f6f3a1ea207be47300b8` y coincide. Node v24.20.0. Corrí cada comando yo mismo, con ejecución fresca.

## Criterios de aceptación, verificados por mí
| # | Criterio | Comando que corrí | Resultado |
|---|---|---|---|
| 1 | Instala, compila y pasa calidad | `npm ci --no-audit --no-fund && npm run lint && npm run typecheck && npm run build; ls dist/index.html` | pasa: `rc=0`, `dist/index.html` existe |
| 2 | Versiones exactas y dependencias acotadas | el `node -e` del criterio | pasa: `0 react,react-dom,react-router-dom` |
| 3 | Tipos del contrato al día | `check:api`, luego deriva (comentario más `getRunX`), luego `git checkout` del contrato | pasa: `rc=0` y `rc_deriva=1` |
| 4 | Cliente API | `npx vitest run src/api` | pasa: 10 de 10 |
| 5 | Proxy por prefijo | `npx vitest run src/proxy` | pasa: 8 de 8 |
| 6 | CI y alcance | los comandos literales | pasa, ver abajo |

- **CA-3 (deriva):** repetí la deriva por separado. El fallo viene del `diff` y no del generador: `schema.gen.ts desactualizado: ejecuta npm run gen:api`. Tras restaurar el contrato, `git status --short` da 0 líneas y no queda `.schema.check.ts`.
- **CA-4 (cobertura):** los seis puntos exigidos están cubiertos.
  - Authorization solo con token.
  - X-Request-Id con el patrón y distinto en cada llamada.
  - 4xx → `ApiError` con code, message y status.
  - 429 con `Retry-After: 7` → `retryAfter: 7`.
  - Cuerpo no JSON → `unexpected_response`.
  - Exactamente 1 petición por llamada, también cuando falla.
  - Además hay una prueba de `network_error` con `status: 0`.
  - MSW usa `onUnhandledRequest: 'error'` y las pruebas tardan 37 ms con 10 pruebas reales, así que no se saltaron.
- **CA-5 (proxy, comprobación más allá del criterio):** sin una variable, `vite` (dev) y `vite preview` fallan con `falta la variable de entorno AQS_UI_API_URL (...)` y `AQS_IDENTITY_URL`. Con tres backends de eco y `vite preview` de verdad, el enrutado fue correcto:
  - `/api/auth/login` → `ID /auth/login`
  - `/api/runs/x` → `RUN /runs/x`
  - `/api/notifications` y `/api/warm` → `UI /notifications` y `UI /warm`
  - `/api/authz` → `UI /authz` (casa por segmento)
  - `/runs/abc` → `index.html` del SPA
- **CA-6 (CI y alcance):** `2` y `0` en los dos primeros `grep`; los siete valores son `1 1 1 1 1 1 1`; `git status --short | wc -l` da `0`. Contra `docs/u6-tareas`, `git diff --name-only docs/u6-tareas HEAD | grep -v <filtro> | wc -l` da `0`: 21 archivos, todos dentro de `web/dashboard/`, `web.yml`, `README.md` y `bitacoras/U6-T01.md`. Contra `origin/main` salen los 9 archivos del commit base `5e91cae` (`aidlc-docs/audit.md`, `tareas/**`, `unidades-y-tareas.md`), como anticipaste. Las acciones usan los mismos SHA que `ci.yml`; `permissions: contents: read`; `paths` en `web/**`, `contracts/openapi/**` y el propio workflow.
- **`npm audit --omit=dev --audit-level=high`:** `rc=0`. Quedan 2 avisos moderados de `react-router`; el arreglo exige salto de major y el umbral de CI es `high`, así que no bloquea.

## Arbitrajes del orquestador, verificados
- `react-router-dom` va en `6.30.6` exacto (confirmado en `package.json`; el audit pasa).
- `@types/react@18.3.12` y `@types/react-dom@18.3.1` están como devDependencies exactas.
- Un fallo de red lanza `ApiError {status: 0, code: "network_error"}`. Tiene prueba, y el README lo documenta.

## Evidencia de la bitácora
Las cifras (9 pruebas antes del arbitraje, 10 después, 8 del proxy) coinciden con mis ejecuciones. La bitácora incluye el rojo primero del cliente (`Failed to resolve import "./client"`). También documenta un defecto real que el codificador encontró y corrigió al probar con Vite de verdad: las claves string de Vite casan por prefijo literal, y por eso usa claves regex `^/api/x(/|$)`. Las pruebas de `table.test.ts` cubren esa regresión.

## Hallazgos
No hay ROJO ni NARANJA. No hay desborde de alcance, ninguna prueba omitida y ningún literal de usuario incrustado. Los nombres de variables y prefijos están fijados por la tarea.

### F-01 · AMARILLO · `web/dashboard/src/api/client.ts` (`newRequestId`) · `crypto.randomUUID` solo existe en contextos seguros
`globalThis.crypto.randomUUID()` no está disponible en navegadores sobre HTTP que no sea `localhost`. Por ejemplo, `vite --host` abierto por IP de LAN lanzaría `TypeError` antes de cualquier petición. Una alternativa es un respaldo con `crypto.getRandomValues`. No bloquea.

### F-02 · AMARILLO · `web/dashboard/src/api/client.ts` (`apiFetch`) · `path` absoluto enviaría el Bearer a otro origen
`new URL(path, base)` acepta una URL absoluta, así que `apiFetch('https://otro/...')` mandaría el token fuera. Hoy no hay llamadores y el contrato documenta rutas relativas `/api/...`. Convendría una guarda `path.startsWith('/api/')` o dejar el invariante en un comentario. No bloquea.

### F-03 · AMARILLO · `web/dashboard/src/api/client.ts` (`parseRetryAfter`) · Rama de fecha HTTP sin prueba
Se parsea `Retry-After` como fecha HTTP, pero ninguna prueba cubre esa rama; solo se prueba el entero. El contrato no declara `Retry-After`. Cubrirla con una prueba o quitar la rama.

### F-04 · AMARILLO · `web/dashboard/package.json` · Sin `engines` ni `.nvmrc`
CI usa Node 20 y yo corrí con Node 24. La tarea pide "Node 20+", pero no hay nada que lo fije en el repo, y no verifiqué en Node 20 (`podman` no se usó). Las versiones fijadas son compatibles con Node 20.

## Tareas candidatas (fuera de alcance)
- Subir `react-router-dom` a 7.x para cerrar los 2 avisos moderados (GHSA-wrjc-x8rr-h8h6 y GHSA-337j-9hxr-rhxg), con revisión de ruptura.
- `eslint@9.16.0` emite aviso de versión sin soporte. La tarea fija esa versión; revisarlo en una tarea de mantenimiento.
- Tipar `apiFetch` por ruta y método a partir de `paths` de `schema.gen.ts`. Hoy `T` es un genérico libre y solo se usa `components['schemas']['Error']`.

## Estado del worktree
Limpio (`git status --short` da 0 líneas). El contrato quedó restaurado y no hay procesos ni archivos temporales dentro del repo. No toqué `tarea/U6-T01` ni `agentic-qa-swarm-wt-U6-T01`. Informe: `revisiones/U6-T01/ronda-1.md` (no lo escribí por la restricción de solo lectura; este texto es el informe).

VEREDICTO: VERDE
AMARILLO|web/dashboard/src/api/client.ts|crypto.randomUUID solo en contextos seguros (F-01); path absoluto filtraría el Bearer (F-02); rama Retry-After fecha sin prueba (F-03); sin engines/.nvmrc (F-04)
INFORME: revisiones/U6-T01/ronda-1.md
