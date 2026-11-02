# Evidencia de Verificación — Onboarding de Proyectos y Selector en Inbox

**Fecha**: 2026-11-01  
**Plan de Implementación**: `aidlc-docs/construction/plans/project-onboarding-code-generation-plan.md`  
**Estado**: Completado y Verificado (Pasos 1 al 8)

---

## 1. Resumen Ejecutivo

Se completó la implementación y verificación del corte vertical de autoservicio de onboarding de proyectos y selector en inbox:
1. **Contratos y Metadatos de Proyecto**: Metadatos extendidos (`name`, `description`, `resources`, `webhook_secret_version`, `infrastructure_ready`), esquemas de rotación de credenciales (`RepositoryCredentials`, `/repositories/{owner}/{repo}/webhook-secret/rotate`), y evento `webhook.validated`.
2. **Aprovisionamiento Resource-Aware**: `go-warm-manager` crea dinámicamente solo los recursos seleccionados (PostgreSQL 16.4 y Redis 7.4.1), omitiendo `DATABASE_URL` y workloads cuando no correspondan, y señalando `infrastructure_ready` sin declarar `reset_verified` prematuramente.
3. **Reset Verificado Resource-Aware**: `go-reset` inspecciona `warm-policy` para ejecutar adaptadores no-op/stateless en recursos omitidos, manteniendo verificaciones estrictas para recursos seleccionados.
4. **Secretos Webhook Aislados y Ping Firmado**: Derivación criptográfica HMAC-SHA256 (`whsec_` + 64 caracteres hex) aislada por proyecto sin persistir secretos crudos, validación fail-closed de pings en `go-intake`, y proyección en `ui-api`.
5. **Entrega y Rotación de Credenciales**: Endpoint `POST /repositories` y `POST /repositories/{owner}/{repo}/webhook-secret/rotate` con entrega de credencial de un solo uso sin exposición en logs, listados o almacenamiento del navegador.
6. **Selector Nativo de Proyectos en Inbox**: Control nativo `<select id="inbox-project-selector">` con etiqueta visible, icono SVG de GitHub, soporte para `/inbox?project=owner%2Frepo`, filtrado sincronizado de notificaciones, conteos de pestañas y estado del entorno warm, y opción de navegación a `/repositories/new`.
7. **Wizard de Onboarding en 3 Pasos y Consolidación**: `/repositories/new` con validación semántica accesible, tarjetas de recursos habilitadas (Postgres/Redis) e ilustrativas deshabilitadas (MySQL/MongoDB "Próximamente"), prellenado de GHCR, entrega de secreto HMAC, instrucciones de eventos GitHub (push, PR, release), sondeo en vivo con `usePolling` y banner final hacia `/inbox?project=...`. Consolidación de `/repositories`.
8. **Verificación Completa y Navegador Multi-Viewport**: Ejecución exitosa de contratos, servicios Go, dashboard web (301 pruebas unitarias/integración, linting, api check, build de producción), RBAC/VAP de autoservicio en Kind, y suite E2E de navegador con Playwright en 6 combinaciones (1440px, 390px, 375px en temas claro y oscuro).

---

## 2. Resultados de Pruebas Unitarias y Contratos

### 2.1 Validación de Contratos JSON Schema y OpenAPI
Comando: `bash contracts/validate.sh`  
Resultado: **EXITOSO (0 errores)**
- 15 ejemplos válidos validados contra sus esquemas (incluyendo `webhook.validated.json`).
- 26 ejemplos inválidos rechazados correctamente por violación de esquema (incluyendo `webhook.validated.secret.json` con campo secreto prohibido).

### 2.2 Servicios Go (Control Plane)
Comando: `go test -count=1 ./...` en cada servicio  
Resultado: **100% PASADO**

| Servicio | Paquetes Probados | Tiempo | Resultado |
|---|---|---|---|
| `services/go-governance` | `authz`, `cmd`, `audit`, `auth`, `gen`, `obs`, `policy`, `repos`, `server`, `service` | 0.28s | **PASS** |
| `services/go-warm-manager` | `warmtarget`, `adapters/kube`, `adapters/objstore`, `adapters/outbox`, `cmd`, `api`, `provision` | 3.84s | **PASS** |
| `services/go-reset` | `adapters/kube`, `adapters/outbox`, `adapters/redis`, `adapters/script`, `adapters/sessions`, `config`, `core`, `httpapi`, `jobspec` | 0.79s | **PASS** |
| `services/go-intake` | `cmd`, `contract`, `artifact`, `gen`, `githubsig`, `intake`, `obs` | 1.48s | **PASS** |
| `services/ui-api` | `cmd`, `inbox`, `auth`, `gen`, `httpapi`, `obs` | 8.94s | **PASS** |

### 2.3 Dashboard Web (`web/dashboard`)
Comando: `npm test -- --run && npm run lint && npm run check:api && npm run build`  
Resultado: **100% PASADO**

- **Pruebas Vitest**: 25 archivos de prueba, 301 pruebas ejecutadas, **301 pasadas** (0 fallas).
  - `src/pages/Inbox/InboxPage.test.tsx`: 49 pruebas pasadas (incluye selector de proyectos, filtrado por query param, navegación a nuevo proyecto, tolerancia a fallos).
  - `src/pages/Repositories/ProjectWizardPage.test.tsx`: 4 pruebas completas pasadas (validación paso 1, recursos paso 2, envío paso 3, entrega/rotación de secreto y sondeo en vivo paso 4).
  - `src/pages/Repositories/RepositoriesPage.test.tsx`: 9 pruebas pasadas (navegación a `/repositories/new`, insignias de recursos y nombres).
- **ESLint**: 0 errores, 0 advertencias.
- **Sincronización de Tipos OpenAPI (`check:api`)**: `schema.gen.ts` sincronizado con `contracts/openapi/control-plane.yaml`.
- **Compilación TypeScript y Vite (`npm run build`)**: Generación de assets limpia en `dist/` (JS 270.96 kB, CSS 67.25 kB).

---

## 3. Verificación de Seguridad y Entorno Kind

### 3.1 Matriz RBAC
Comando: `bash scripts/ci/rbac-matrix.sh`  
Resultado:
```
OK rbac-matrix dev
OK rbac-matrix prod
```

### 3.2 Autoservicio E2E en Cluster Kind
Comando: `bash scripts/test/u12-self-service-kind.sh`  
Resultado:
```
== U12 E2E :: Autoservicio de Repositorios & Entornos Warm ==
--- 1. Verificando RBAC para go-warm-manager ---
OK: go-warm-manager puede crear namespaces
OK: go-warm-manager puede crear secretos (create)
OK: go-warm-manager NO puede leer secretos (get rechazado)
OK: go-warm-manager NO puede listar secretos (list rechazado)
--- 2. Verificando ValidatingAdmissionPolicy (confinamiento a aqs-w-*) ---
OK: namespace fuera de aqs-w-* rechazado por VAP
OK: namespace aqs-w-e2e-demo creado exitosamente por go-warm-manager
--- 3. Verificando ciclo de vida de recursos en aqs-w-e2e-demo ---
OK: Secret y ConfigMap creados en aqs-w-e2e-demo
OK: namespace aqs-w-e2e-demo eliminado exitosamente por go-warm-manager
--- 4. Verificando integridad de aqs-test ---
OK: namespace aqs-test presente y activo
== U12 E2E :: TODOS LOS CHECKS PASARON EXITOSAMENTE (0 FALLAS) ==
```

### 3.3 Verificación de Humo Kind (`scripts/kind/kind-smoke.sh`)
- 22 comprobaciones de control plane, healthz, readyz, login, inbox autenticado/no-autenticado, warm, RBAC, netpol sin egress y LLM pasadas.
- Restablecida imagen operativa de `warm-app` en `aqs-test` (`ghcr.io/ogaston/demo-target-app-qa-swarm:b97a719199d5ccbc2ff24f9cfc408ec7ee31114d`).
- Nota: `ciclo-webhook-confirm-corrida` depende del despliegue en vivo de las imágenes de controlador que no se regeneran en este paso para respetar la restricción estricta de **no deploy / no commit**.

---

## 4. Verificación E2E de Navegador Multi-Viewport (Playwright)

Comando: `node web/dashboard/scripts/e2e-project-onboarding.mjs` (registrado en `npm run e2e:onboarding`)  
Build testeado: `vite build` en modo producción servido localmente.

### Matriz de Combinaciones Ejecutadas:

| Viewport | Resolución | Tema | Desbordamiento Horizontal | Errores de Consola / Página | Resultado |
|---|---|---|---|---|---|
| `desktop-1440` | 1440 × 900 | Claro | 0 px | 0 | **PASS** |
| `desktop-1440` | 1440 × 900 | Oscuro | 0 px | 0 | **PASS** |
| `mobile-390` | 390 × 844 | Claro | 0 px | 0 | **PASS** |
| `mobile-390` | 390 × 844 | Oscuro | 0 px | 0 | **PASS** |
| `mobile-375` | 375 × 667 | Claro | 0 px | 0 | **PASS** |
| `mobile-375` | 375 × 667 | Oscuro | 0 px | 0 | **PASS** |

### Flujos Verificados en Cada Combinación:
1. **Inbox & Selector**:
   - Renderizado de `#inbox-project-selector` con etiqueta accesible "Proyecto:".
   - Opciones: "Todos los proyectos", repositorios individuales, y "+ Añadir nuevo proyecto".
   - Filtrado reactivo de notificaciones y actualización de URL (`/inbox?project=acme%2Fportal-pagos`).
   - Selección de opción `__new__` navega a `/repositories/new`.
2. **Wizard Paso 1 (Información y Contratos)**:
   - Validación de campos obligatorios: error ante nombre vacío y requisitos del App Contract.
   - Diligenciamiento de nombre y descripción opcional.
   - Confirmación de puertos HTTP 8080 y publicación en GHCR.
3. **Wizard Paso 2 (Recursos)**:
   - Verificación de tarjetas activables de PostgreSQL 16.4 y Redis 7.4.1 (seleccionadas por defecto).
   - Verificación de tarjetas deshabilitadas de MySQL 8.4 y MongoDB 7.0 con distintivo "Próximamente".
   - Conmutación independiente de recursos (permitiendo aplicaciones stateless).
4. **Wizard Paso 3 (Repositorio GitHub)**:
   - Validación de formato estricto `owner/repo`.
   - Prellenado automático del prefijo de imagen GHCR (`ghcr.io/owner/repo`).
   - Bloqueo de botón de envío hasta que el formato sea válido.
   - Envío atómico mediante `POST /api/repositories`.
5. **Wizard Paso 4 (Credenciales y Verificación en Vivo)**:
   - Visualización de URL de webhook y secreto derivado `whsec_...`.
   - Copia al portapapeles de URL y Secreto con retroalimentación visual ("Copiado").
   - Rotación del secreto mediante botón "Rotar secreto", actualización a versión incrementada y generación de nuevo secreto.
   - Sondeo reactivo con `usePolling` de infraestructura y recepción de eventos GitHub.
   - Transición exitosa al banner final "✓ ¡Proyecto conectado y verificado exitosamente!".
   - Enlaces directos hacia `/inbox?project=...` y `/repositories`.
6. **Diseño Responsivo y Ergonomía**:
   - Touch targets mínimos de 48px en controles móviles.
   - Prevención de desbordamiento horizontal en cabecera de navegación (`.az-nav` con scroll horizontal táctil y tabla con `.table-wrap` posicionado).
   - Compatibilidad completa con `prefers-reduced-motion: reduce`.
   - Ningún secreto persistido en almacenamiento del navegador ni expuesto en logs.
