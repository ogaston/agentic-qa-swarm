# Ronda 1 — U1-T02

VEREDICTO: VERDE

Verifiqué el SHA ebabb863733956a8052bbb4bda8e9cc3aa40df30 en rama `tarea/U1-T02`. Corrí CA-1 a CA-8 yo mismo y todos pasan, también CA-7. No quedan hallazgos ROJO ni NARANJA. El worktree quedó limpio (`git status --short` da 0 líneas).

## Criterios de aceptación, verificados por mí
| # | Comando | Resultado |
|---|---|---|
| 1 | `go test -run Webhook -v` contando PASS, y conteo de FAIL | 27 PASS y 0 FAIL. Los 9 casos exigidos están en la tabla `TestWebhook`. |
| 2 | Binario real, `curl` firmado (enviado dos veces con la misma entrega), `ajv` | `202` en ambas, 1 línea de evento, `ev.json valid`, estado `pending`. |
| 3 | Firma `sha256=00` | `401`, 0 archivos en el directorio de datos, `sin eventos`. |
| 4 | `go test -run 'Idempotent\|Restart'` | PASS en `TestWebhookIdempotentDelivery`, `TestWebhookRestartKeepsDeliveryIndex` y `TestStoreRestartIgnoresBrokenLine`. |
| 5 | `go list -deps` y `grep` de k8s | `0` y `0`. |
| 6 | Sin secreto, y con secreto más `grep` en el log | `rc=1` y `0`. El log solo tiene la línea de escucha y `status=401 code=invalid_signature`. |
| 7 | Imagen, `FROM`, `list-services`, `detect-lang` | Construye, corre como UID `65532` con `EXPOSE 8080`. Ningún `FROM` sin tag o con `latest`; `list-services` da `1` y `detect-lang` da `go`. |
| 8 | `vet`, `gofmt`, `git status`, diff fuera de alcance | `ok`, `0` y `0`. |

Adicionalmente, un cuerpo de 2 MB con firma válida responde `413`. `go test -count=1 ./...` pasa completo, sin saltos.

## Sobre el `docker build` de CA-7
La afirmación del codificador es cierta solo a medias. No hay daemon en `/var/run/docker.sock`, pero `dockerd` está instalado y sí arranca (`--exec-root /run/rv -H unix:///run/rv/d.sock --iptables=false --bridge=none`; las rutas del scratchpad fallan porque el socket de containerd supera los 104 caracteres). Con el daemon arriba, construí el Dockerfile en un contexto temporal fuera del repo, con `ca-bundle.crt` añadido como indica la nota de la tarea: la imagen se construyó; `Config.User` es `65532:65532`; `ExposedPorts` es `8080/tcp`; el contenedor arrancó y escuchó en `:8080`; `/var/lib/go-intake` pertenece al usuario `intake`. Por inspección, el Dockerfile sigue el patrón de `services/go-identity`: mismos tags, dos etapas, binario estático con `CGO_ENABLED=0`; solo añade el directorio de datos con dueño 65532 y los `ENV` de `INTAKE_*`.

## Seguridad
- **Firma antes de parsear:** `HMACVerifier` usa `hmac.Equal` sobre el cuerpo crudo, en tiempo constante. Rechaza prefijo ausente, hex inválido, longitud incorrecta y secreto vacío. Una cabecera ausente da `401` sin leer el cuerpo.
- **Límite de cuerpo:** `MaxBytesReader` a 1 MiB; el `413` se comprobó con el binario real.
- **Secreto:** nunca se loguea; el servicio no arranca si falta.
- **Orden de comprobaciones:** primero firma, luego `Content-Type`, `X-GitHub-Delivery` y clasificación.
- **Idempotencia:** mutex global; el índice por entrega sobrevive a un reinicio. Un rechazo no persiste ni publica.
- **Kubernetes:** no se importa nada de k8s.

## Hallazgos
- **F-01 · AMARILLO · bitácora, CA-7:** dice que no se pudo construir por falta de daemon. En realidad se podía arrancar uno aparte. La verificación real de la imagen la hizo el revisor.
- **F-02 · AMARILLO · `handler.go`, ruta de publicación:** si `Publish` tiene éxito pero el `Put` posterior que limpia `PublishPending` falla, el reintento publica un segundo evento. Duplicado at-least-once en una ventana mínima, aceptable con un outbox de transición.

## Tareas candidatas
- Ya propuesta por el codificador: `release.published` trae `target_commitish` y no un SHA, así que hoy responde `400 invalid_payload`. Debe resolverse en U1-T03 (ya registrada como C-75).
- Hacer el contenedor compatible con rootfs de solo lectura y volumen montado para los datos en `/var/lib/go-intake`. Es de manifiestos y está fuera de alcance.

VEREDICTO: VERDE
AMARILLO|bitacoras/U1-T02.md CA-7|La bitácora afirma que no hay Docker, pero un daemon sí arranca: el build y el usuario 65532 se verificaron
INFORME: revisiones/U1-T02/ronda-1.md
