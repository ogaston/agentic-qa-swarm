# Ronda 2 — U11-T05 (informe del revisor, persistido por el orquestador; resumen fiel del hand-back)

VEREDICTO: NO-VERDE (por un NARANJA de cobertura en la prueba; el código y CA-1..CA-4 pasan)

Hash de la tarea tras el último commit de docs: 20aa0625… (el revisor no recibió un hash esperado; la bitácora registra el anterior 9a1f8d2…, de antes de la corrección de CA-4). Worktree limpio. Scratchpad único.

## Criterios (ejecución propia)
CA-1 `bash scripts/test/u11-egress-port-test.sh`: 14 OK + OK, rc=0. CA-2: SINTAXIS_OK, `u.port or 80` → 0, diff de kind-smoke.sh 10 inserciones / 4 borrados solo en `llm_egress_acotado`. CA-4: 0/0/0.
CA-3 reproducido por el revisor en un clúster real (2026-10-25 22:24, posterior al commit): `OK llm-egress-acotado` en 3 corridas; sonda del puerto 80 desde el pod del planner → PUERTO80_BLOQUEADO; cuarta corrida del humo → 0 FALLA, 25 controles OK; puerto derivado del pod real 443, esquema https; logs del bring-up y del humo con 0 ocurrencias del host y de la URL (medido con `grep -cF`). Discriminación en clúster (copia con `port=80` en la función) → `FALLA llm-egress-acotado` y `smoke: FALLA`: el control no se debilitó. Clúster bajado.

## Mutaciones sobre `llm_endpoint_host_port`
Detectadas: regla vieja (en la prueba y pegada en la función; falla `https://h` y 6 más), https→80, http→443, ignorar el puerto explícito, no descartar credenciales, aceptar `ftp://`, quitar la validación de rango (`99999`), código de salida 2 en vez de 1, host vacío con regex `*`.
SOBREVIVEN: puerto 0 aceptado (`>= 0`) y puerto 65536 aceptado (`<= 65536`) con rc=0; quitar solo `-n "$host"` es un mutante equivalente (la regex `[^:[]+` ya exige host no vacío).
Ataque directo: nunca imprime la URL ni las credenciales (ni en éxito ni en error); sin `eval` ni backticks; host y puerto llegan al Python del pod como argv. URLs raras: `https://u:p@h:8443/v1` → `h 8443`; `https://[::1]:8443/` → `::1 8443`; `https://h:`, `:abc`, `:8443path`, `:1:2/`, `:+443`, `https://u:p@/` → rc=1; `:0`, `:65536`, `:99999999999999999999` → rc=1; `:08` → `h 8`; `:65535` ok.

## `llm_egress_acotado`
Lee `LLM_BASE_URL` del pod sin imprimirla; el control negativo `1.1.1.1:443` intacto; si la URL está vacía o mal formada el control devuelve 1 (no hay vía por la que un fallo de la función lo haga pasar). Política no tocada (0 cambios en deploy/secrets/services/agents; 0 `hostAliases`). Compatibilidad con U11-T02: merge-base eca4e34; solo añade 27 líneas a lib.sh y borra 0; `u11-scripts-test.sh` termina en OK.

## Hallazgos
### F-01 · NARANJA · scripts/test/u11-egress-port-test.sh
Los límites del rango de puerto no están cubiertos: las mutaciones que aceptan 0 o 65536 sobreviven. La prueba solo comprueba `99999`. Faltan `caso_err` para `https://h:0` y `https://h:65536`, y conviene un caso OK para `https://h:65535`. Riesgo operativo nulo (la conexión fallaría igual), pero es una regla documentada sin cobertura.

### F-02 · AMARILLO · scripts/kind/lib.sh:345-349
La validación del host es laxa (admite espacios, `;`, `$()` y saltos de línea; con `https://h\nevil` imprime `h\nevil 443`) y no admite el esquema en mayúsculas (`HTTPS://H` → rc=1 aunque `urlsplit` lo acepta). Efecto fail-closed (read -r toma la primera línea; `int('')` falla en el pod); sin ejecución de comandos. Sugerencia: host `[A-Za-z0-9._-]+` y esquema en minúsculas.

### F-03 · AMARILLO · scripts/test/u11-egress-port-test.sh:77-82
La sensibilidad depende de que `python3` exista: si no está, `regla_vieja` devuelve vacío y el bloque imprime `OK sensibilidad` sin detectar nada. Arreglo: exigir que `got_viejo` sea exactamente `h 80`. Además el par falla-pasa pegado en la bitácora es esa comprobación aparte, no la prueba completa contra la regla vieja (que sí falla, según el revisor).

### F-04 · AMARILLO · bitacoras/U11-T05.md
La evidencia de CA-3 de la bitácora no tiene marcas de tiempo ni los comandos completos; la sección «CA-4» dice «ver abajo» y no hay nada debajo.

## Candidatas
Ninguna nueva. El alias de host sigue descartado y solo se reabriría si se observa rotación real de IP.

VEREDICTO: NO-VERDE
