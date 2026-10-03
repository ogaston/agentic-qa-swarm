# Ronda 2 — U4-T02

VEREDICTO: VERDE

Los 11 criterios pasan con la ejecución propia del revisor (`-count=1`). F-01, F-02 y F-03 de la ronda 1 resueltos contra el código real. Sin ROJO ni NARANJA; tres AMARILLO que no bloquean. Worktree limpio antes y después; sondas y mutaciones en copias del scratchpad.

## Arbitraje aplicado
- CA-9 con `keys | sort`.
- CA-10: `docker build` literal falla por la CA del proxy; repetido en un contexto temporal con `COPY ca.crt` y `ENV SSL_CERT_FILE` (diff contra el Dockerfile real: solo esas dos líneas; la CA no llega a la imagen final; en `/usr/local/bin` solo `go-identity`).
- govulncheck no puede correrse aquí (403 de `vuln.go.dev`): pendiente de confirmar en la CI.
- El diff de `revisiones/U4-T02/` no cuenta como desborde.

## Criterios de aceptación, verificados por el revisor
| # | Resultado |
|---|---|
| 1 | pasa: 94 PASS, 0 FAIL, 0 SKIP (guard 6.5 s, server 13.3 s) |
| 2 | pasa: `200`, `43`, fecha futura, `204 401` |
| 3 | pasa: dos líneas idénticas con el mismo `message` |
| 4 | pasa: `401×5`, `429`, `Retry-After: 30` |
| 5 | pasa: `401 mfa_required`, `200`, `401` por reuso |
| 6 | pasa: seis `rc=1` |
| 7 | pasa: gitleaks `0`, grep `0`, log `0` |
| 8 | pasa: 6 PASS |
| 9 | pasa (con `sort`): lint válido, `0` líneas eliminadas, `username,password \| otp,password,username` |
| 10 | pasa con la sustitución arbitrada: `65532:65532`, `0`, `1`, `go`, `0`; `EXPOSE 8080/tcp` |
| 11 | pasa: `ok`, `0`, `0` |

## Verificación de F-01..F-03
- **F-01 (password spraying por IP): resuelto.** Sonda con el binario real (12 víctimas inexistentes con un login correcto de marta intercalado): `401:200` ×4, luego `401:429` y desde ahí `429:429`; en la ronda 1 daba 12 de 12 sin `429`. Intercalar `mfa_required` con fallos también acaba en `429`. Cada acierto es neutro para la IP (`Allow` suma una reserva, `Success` la devuelve; si el acierto dispara el bloqueo, se deshace). Ventana por IP: un atacante sostiene como máximo 4 fallos por ventana de 15 min (límite por diseño); los fallos expiran como dice el README; la escalada se reinicia al expirar la serie (bloqueos medidos 30 s, 60 s, 2 min 30 s, 5 min 30 s y reinicio). Carrera: 200 goroutines mezclando `Allow`/`Success`/`Release` con `-race`, limpio. Las dos pruebas reescritas conservan la intención: revertir `Success` a borrar la IP, quitar la ventana o no devolver la reserva hace fallar pruebas concretas.
- **F-02 (documentación): resuelto.** `services/go-identity/README.md` lista las variables `IDENTITY_*` y `LISTEN_ADDR` con defaults (5, 30m, 15m, 5, false), la advertencia de proxy/IP y los límites de diseño; coincide con `main.go`.
- **F-03 (señuelo con parámetros máximos): resuelto.** Con m=128 MiB, t=5, p=4 el señuelo copia los parámetros (180 ms frente a 186 ms contra el hash real); con los máximos aceptados (1 GiB, t=16, p=32) 5.03 s frente a 4.94 s. Generarlo cuesta ~8.7 s una vez al arrancar. Matiz: quitar la copia hace fallar `TestDecoyCopiesHighestParams`, pero el cableado `server.NewDecoy(us)` solo se verifica por lectura y sonda.

## Hallazgos (todos AMARILLO, no bloquean)
### F-04 · `guard.go:106` y README · Ventana fija desde el primer fallo, con ráfaga en el borde
Caben ~4 fallos justo antes y ~5 justo después de la frontera (~9 en un par de segundos). El comentario de `table.window` dice «sin actividad durante la ventana», que no corresponde al comportamiento; el README sí dice «fija».

### F-05 · README, sección de proxy · «Escalada hasta 15 min» no se alcanza por IP
Por la ventana, el bloqueo por IP no pasa de ~5 min 30 s; el tope de 15 min solo es alcanzable por usuario.

### F-06 · `internal/server` · Cobertura del cableado del señuelo
Ninguna prueba detecta que `NewDecoy` se llame con `nil` o con la lista equivocada.

## Tareas candidatas (fuera de alcance)
- Limitar la concurrencia de argon2 y poner `resources.limits` de memoria al Deployment.
- Añadir a C-49 que el estado del guard también se pierde por réplica (límite efectivo ×N).
- Que la CI de imágenes confíe en la CA del entorno, o documentar la sustitución de CA-10.
- Confirmar govulncheck en la CI real.

VEREDICTO: VERDE
AMARILLO|services/go-identity/internal/guard/guard.go:106|Ventana por IP fija desde el primer fallo: ráfaga en el borde y comentario impreciso
AMARILLO|services/go-identity/README.md (sección proxy)|«Escalada hasta 15 min» no se alcanza por IP (la serie se reinicia antes)
AMARILLO|services/go-identity/internal/server|Ninguna prueba detecta el cableado del hash señuelo con parámetros altos
INFORME: revisiones/U4-T02/ronda-2.md
