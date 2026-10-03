# Respuesta a la ronda 1 — U4-T02

F-01: Corregido en el commit de esta ronda (ver `git log` de tarea/U4-T02): `Success` reinicia solo el contador del usuario, el de la IP cuenta fallos y decae por ventana fija de 15 min (`guard.IPWindow`); rojo y verde con prueba unitaria (reloj falso) y binario real (12 víctimas: antes `401 ok:200` x12, ahora `429` tras el quinto fallo), en la bitácora.
F-02: Corregido en el commit de esta ronda: `services/go-identity/README.md` con todas las variables IDENTITY_*, defaults, advertencia de proxy/IP y límites de diseño.
F-03: Corregido en el commit de esta ronda: el hash señuelo copia los parámetros m, t, p más altos de los hashes cargados (`passhash.Decoy`, con prueba); los demás puntos (sesiones/guard por réplica, reservas por adelantado, argon2 sin límite de concurrencia) quedan documentados en el README y como candidatas en la bitácora.
