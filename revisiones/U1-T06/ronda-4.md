# Ronda 4 — U1-T06

VEREDICTO: VERDE

Los nueve CA dan lo esperado y F-01, F-02, F-03 y F-05 quedan cerrados: todos los mutantes de la ronda 3 mueren. No hay NARANJA. Quedan AMARILLOS menores que son candidatas y no defectos de la tarea.

Trabajé sobre `git archive HEAD` en `scratchpad/rev-u1t06-r4/`. El worktree sigue en `53d8b29` con 0 cambios y no quedan procesos vivos. Todo se ejecutó con `GOTOOLCHAIN=go1.26.8` y `-count=1`.

## Criterios de aceptación
| CA | Resultado |
|---|---|
| 1 | go-intake 156 PASS / 0 FAIL; ui-api 217 / 0. |
| 2 | `healthz 200`, `readyz 200`; tras borrar `data_dir` (soy root): `{"checks":{"data_dir":"fail",...},"status":"unavailable"} 503`. |
| 3 | `true` y `4bf92f3577b34da6a3ce929d0e0e4736`. |
| 4 | `0`. |
| 5 | `2`, `0`, `1`, `36`. |
| 6 | 11 pruebas `TestOpsEndpoints*` más `TestOpsEndpointsNeedNoTokenAndSkipApp` en PASS; todo `ok`. |
| 7 | Sin cambios: `git diff a62e7a1 HEAD -- deploy` está vacío. Lo dado en la ronda 3 sigue vigente, incluido el `FALLA kubeconform` por falta de red. No lo reejecuté. |
| 8 | Sin cambios en `dashboard-u1.yaml`; vale lo de la ronda 3. No lo reejecuté. |
| 9 | `go-intake ok`, `ui-api ok`, `0` en el estado del worktree. El diff contra `dfff676` solo sale de alcance por `revisiones/` (informes del loop, que la tarea excluye). `kustomization.yaml`: `0` líneas extra. |

## Refactorizaciones de producción
- **`newLog` (ambos servicios):** es idéntico a las dos líneas anteriores de `main()`: `NewLogger(w, serviceName, ParseLevel(LOG_LEVEL))` más `SetDefault`. Solo cambia el destino, ahora un parámetro.
- **`newSubscriber` (ui-api):** misma expresión que antes (`slog.NewLogLogger(log.Handler(), LevelInfo)`) y `run` lo llama.
- **`cfg.Logger` (ui-api):** confirmado muerto. `h.log` solo se usa en `logError` cuando `cfg.Slog == nil`, y `newHandler` siempre pone `Slog`.
- **Sin otras ediciones de producción:** el diff entre `a62e7a1` y `f7151c5` toca solo esos dos `main.go`.
- **Pruebas:** no se debilitó ninguna. La única línea eliminada en `_test.go` es `r.ctype = "text/plain"`, sustituida por una variante con marca de PII (más estricta).
- **Secretos y PII:** no hay en el diff nuevo. Las marcas son `falso.test` y `PII-*`.

## Mutantes
Unos 330 propios, ejecutados en copias del scratchpad. Cubren:
- cableado de ambos `main` (servicio, ruta, registros, `in_flight`, `ReadyChecks`, `cfg.Slog`);
- `ParseLevel`, `SetDefault` y el nivel del suscriptor;
- niveles y campos de los logs de dominio;
- 60 mutantes de log de acceso con cabeceras, ejecutados en cada servicio;
- `Connection: close` (9 variantes, ambos servicios), `in_flight` y rutas;
- motivos de rechazo, series `reason="other"` y redacción de claves sensibles.

Todos los supervivientes de la ronda 3 mueren: `serviceName`, `Wrap.Service`, `Route: nil`, etiqueta `service` de `in_flight`, `ParseLevel("")`, quitar `SetDefault`, niveles del suscriptor y de los logs de dominio, `notification_id`, `status` fijo, serie `reason="other"`, `Cookie` y las cabeceras de PII, y `Connection: close` fuera del 413.

Equivalentes o fuera de alcance, ya descartados:
- `ReadyChecks` con un secreto no vacío y `verifierSet` siempre verdadero: invariantes de arranque.
- `ua-sub-handler`: usa `slog.Default()`, que es el mismo logger tras `SetDefault`.
- Campo `github_event` y `rec.Artifact` en los logs: no son PII.
- Quitar el valor por defecto 200 de `statusWriter`: inalcanzable, porque ningún handler deja de escribir.
- `NewArtifactResolver("")`, `LISTEN_ADDR`, `UIAPI_TRUST_PROXY` y `RATE_LIMIT` por defecto: configuración de `run()` anterior a esta tarea, sin relación con observabilidad.
- Mutantes que no compilaron o eran no-op (`ua-panic-rec`, `reject-body`, `gi-verifier`, `ua-wrap-outside`, `ua-confirm-level`): no cuentan.

## Hallazgos
| ID | Severidad | Detalle |
|---|---|---|
| F-07 | AMARILLO (candidata, no defecto) | Una prueba de lista cerrada de claves del log de acceso (`request`) cerraría toda la clase de «loguear cabecera X». Sin ella, sobreviven mutantes que loguean cabeceras que las pruebas no envían, en ambos servicios: `Forwarded`, `X-Forwarded-Host`, `X-Api-Key`, `X-Csrf-Token`, `tracestate`, `Accept`, `Accept-Encoding`, `Sec-Fetch-Site`, `X-HTTP-Method-Override`, `X-GitHub-Event` y `X-GitHub-Delivery`. Solo `Forwarded`, `X-Forwarded-Host`, `X-Api-Key` y `X-Csrf-Token` son realmente sensibles. La lista de cabeceras es abierta. |
| F-08 | AMARILLO-bajo (candidata) | `clip(delivery)` no se prueba en los sitios posteriores a la firma: `persist` ×2, `publish` y `accept`. Solo se prueba la función suelta y el rechazo. Hay que tener firma válida para llegar ahí, y el valor sigue escapado por JSON. |
| F-09 | AMARILLO-bajo (candidata) | ui-api: el log del panic con `r.URL.String()` sobrevive, porque la prueba del panic no mete marca en la ruta. Las rutas de ui-api solo llevan ids. |
| F-10 | AMARILLO-bajo (candidata) | Niveles y mensajes de `run()` sin prueba, como `log.Warn("registro de confirmaciones...")` pasado a Debug. `run` no es testeable sin arrancar el servicio. |

F-04 sigue arbitrado fuera de alcance (C-82): ni lo reabrí ni lo mutué.

## CI del PR #35 (borrador, abierto) sobre `53d8b29`
- Los runs `ci`, `policies` y `contracts` del PR terminaron en `success`. También los de push de `policies` y `contracts`.
- `build`, `test`, `vuln` y `sbom` de go-intake, ui-api, go-identity y go-governance: `success` (23 check-runs completados). Ya no hay cancelaciones: F-06 queda resuelto.
- `publish` está `skipped`, como corresponde a un PR en borrador.

VEREDICTO: VERDE
