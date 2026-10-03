# Ronda 3 — U4-T02

VEREDICTO: VERDE

Revisión posterior al fallo del CI de GitHub (`vuln` / govulncheck sobre la stdlib de Go 1.24.x) y a los cambios de toolchain y dependencias. HEAD efd2512, con `GOTOOLCHAIN=go1.26.8`, todo con `-count=1`. El delta `6a9a380..HEAD` solo toca `go.mod`, `go.sum`, `Dockerfile`, `README.md`, la bitácora y `revisiones/U4-T02/ronda-2-respuesta.md`; no hay cambios de código Go. Worktree limpio tras la revisión.

## Criterios de aceptación, verificados por el revisor
CA-1: 94 PASS, 0 FAIL, sin saltos (~13 s). CA-2: `200`, `43`, fecha futura, `204 401`. CA-3: dos respuestas idénticas. CA-4: `401×5`, `429`, `Retry-After: 30`. CA-5: `mfa_required`, `200`, `401` por reuso (OTP calculado con Python). CA-6: seis `rc=1`. CA-7: gitleaks 0, grep 0, log 0. CA-8: 6 PASS. CA-9: lint válido, 0 líneas eliminadas, `username,password | otp,password,username` (con `keys | sort`). CA-10 (contexto temporal con la CA del proxy): `65532:65532`, 0, 1, `go`, 0; `alpine 3.24.2`; los dos `FROM` con tag fijado y existentes; binario `go1.26.8`. CA-11: `ok` y 0 en `git status`; la tercera cifra da 4 por los informes de `revisiones/U4-T02/` (artefactos del loop, excluidos desde la ronda 2).

## Seguridad tras el cambio de dependencias
- Compatibilidad de hashes entre x/crypto 0.31.0 y 0.57.0: se compilaron ambos y se probaron cruzados; mismo formato PHC (`$argon2id$v=19$m=19456,t=2,p=1$…`); cada binario aceptó los hashes del otro y rechazó la contraseña mala. Parámetros mínimos sin cambios.
- Anti-brute-force con binario real: rociado de 7 usuarios desde una IP con `X-Forwarded-For` falso (ignorado): `401×5`, luego `429 429`; la contraseña correcta también recibe `429`; un acierto no reinicia el contador por IP; las variantes de mayúsculas suman al mismo contador; un usuario inexistente se bloquea igual.
- La imagen sigue sin root (65532).

## Veracidad de las respuestas del codificador
- F-07 «Corregido en 13f9fa0»: confirmado por el CI (run `ci` 37148046566 en success, incluido `vuln`). El run 37147855148 sobre 5627b1c (Go 1.24.13) había fallado en `vuln`, así que «1.24.13 no bastaba» es veraz. Los runs `ci`, `contracts` y `policies` sobre efd2512 terminaron en success.
- F-04, F-05, F-06 (AMARILLO de la ronda 2): «No bloquea, sin cambio» es veraz.

## Hallazgos
### F-08 · AMARILLO · CA-9 y `yq` · el comando literal ya no coincide con la salida esperada
Sin `sort`, `yq` devuelve `username,password,otp`. Defecto de redacción del criterio, no del código.

## Tareas candidatas (fuera de alcance)
- `go-governance` sigue en `go 1.24`: su `vuln` fallará igual que el de este servicio (actualizar en U4-T04).
- Limitar la concurrencia de argon2 y poner `resources.limits` de memoria al Deployment.
- C-49: anotar que el estado del guard también se pierde por réplica.

VEREDICTO: VERDE
AMARILLO|CA-9 (yq sin sort)|El comando literal del criterio no coincide con la salida esperada (ya anotado en la ronda 2)
INFORME: revisiones/U4-T02/ronda-3.md
