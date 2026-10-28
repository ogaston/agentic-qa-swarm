# Demo en vivo — scripts asistidos

Tres scripts que llevan la demo de `docs/demo-real.md` con el mínimo de pasos manuales.
Guion completo y contexto: `docs/demo-real.md`. Requisito previo del humano: webhook
configurado en el repo objetivo (`settings/hooks`, evento `push`, JSON, secreto = el de
`go-intake-webhook`) y clon local de `ogaston/demo-target-app-qa-swarm` con push a `main`.

## Flujo

```bash
# 0. Dejar el entorno listo (clúster, despliegue, secretos, túnel, ghcr, humo)
bash scripts/demo/00-setup.sh            # o con --deploy si la plataforma no está desplegada

# 1a. Commit sano: edita nada, empuja y espera la imagen en GHCR
bash scripts/demo/10-push-commit.sh --repo-dir /ruta/al/clon --bug off
# 1b. Conducir la corrida y ver el resultado (veredicto, reporte, reset)
bash scripts/demo/20-run-and-watch.sh <SHA>     # el SHA lo imprime el paso anterior

# 2a. Commit con el defecto oversell (añade ENV TARGET_APP_BUG=oversell al Dockerfile)
bash scripts/demo/10-push-commit.sh --repo-dir /ruta/al/clon --bug oversell
# 2b. Misma corrida; esperado: veredicto bug citando la invariante de stock
bash scripts/demo/20-run-and-watch.sh <SHA>
```

En `20-run-and-watch.sh` la confirmación la hace el humano en el dashboard por defecto
(el script espera y detecta el recibo); con `--confirm-auto` la hace el script.

## Qué hace cada uno

| Script | Responsabilidad | Salida clave |
|---|---|---|
| `00-setup.sh` | Precondiciones del Paso 0: clúster `kind-aqs`, despliegue, 14 Secrets, túnel `cloudflared`, red a ghcr.io, `kind-smoke.sh` sin FALLA. Imprime la URL pública del webhook y la del dashboard | `OK`/`FALLA` por comprobación |
| `10-push-commit.sh` | Edición idempotente del Dockerfile (añadir/quitar `ENV TARGET_APP_BUG=oversell`), commit, push a `main` y espera de la imagen `ghcr.io/…​:<sha>` | `SHA=<sha40>` + `OK imagen-publicada` |
| `20-run-and-watch.sh` | Webhook → notificación → confirmación → vigilancia de fases hasta `done` → resumen (imagen desplegada, veredicto, hallazgo, `reset_verified`) | `OK demo-corrida-completa` o `FALLA <fase>` |

## Notas

- Todos respetan el lock `flock ${XDG_RUNTIME_DIR:-/tmp}/aqs-kind.lock` y reusan
  `scripts/kind/lib.sh` (comprobación de imagen en GHCR con token anónimo, contexto kind).
- No crean pods auxiliares ni dejan port-forwards vivos (trap de limpieza).
- Timeouts ajustables por entorno: `ESPERA_IMAGEN_S` (10), `CONFIRM_TIMEOUT_S` y
  `DEMO_RUN_TIMEOUT_S` (20).
- `20-run-and-watch.sh` no acepta `resetting` ni `done` con `fail_reason` como final feliz
  (cautela F-01 de U8-T08).
- La CA-6 (lectura de vuelta con un segundo SHA) ya quedó verificada el 2026-10-26; si se
  repite, basta correr el paso 1 tras el paso 2 y comparar la imagen desplegada.
