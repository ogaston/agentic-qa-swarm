# Ronda 3 (última) — U11-T05 (informe del revisor, persistido por el orquestador; resumen fiel del hand-back)

VEREDICTO: NO-VERDE (tope de 3 rondas: pasa al humano)

Hash de la tarea 20aa0625… (esperado). HEAD 482b90c. Worktree limpio antes y después. Scratchpad único.
CA-1..CA-4 pasan con ejecución propia. CA-3 reproducido en un clúster real por el revisor (2026-10-25 22:41-22:48): bring-up deploy OK; 3 × `OK llm-egress-acotado` (22:43:35, 22:44:34, 22:45:31); sonda del puerto 80 desde el pod del planner → PUERTO80_BLOQUEADO; cuarta corrida del humo: 0 FALLA, 25 OK; puerto derivado 443 (https sin puerto explícito); control negativo `1.1.1.1:443` → NEG_BLOQUEADO; el host de LLM_BASE_URL aparece 0 veces en los logs (grep -cF); clúster bajado.
Regresiones: `bash -n` pasa en todos los scripts; u11-scripts-test.sh OK; 0 cambios en deploy/, services/, agents/ y secrets.sh; el control negativo no está en el diff.

## Hallazgos de la ronda 2
- F-01 (límites de puerto): corregido para los dos mutantes citados (el revisor repitió `>= 0` y `<= 65536`: la prueba falla con cada uno); quedan otros mutantes (F-05).
- F-02 (host y esquema): corregido. Fuzz de 1523 URLs, incluidas 22 inyecciones fijas (`$(touch CANARY)`, backticks, `\n`, `\r`, `${IFS}`, `a[$(...)]`): 298 con rc=0 y 1225 con rc=1; ninguna salida fuera del patrón «host puerto» de dos campos; ningún archivo CANARY; ninguna salida contiene credenciales.
- F-03 (sensibilidad): corregido (exige `h 80`; sin python3 falla explícitamente; la prueba completa contra la regla vieja pegada en la función da 14 FALLA).
- F-04 (bitácora): corregido, con un resto (F-07).

## Barrido de mutaciones (95 mutantes, 55 sondas)
La prueba mató 52. Sobrevivieron 30: 11 equivalentes (la guarda de host vacío es código muerto porque la regex ya exige `+`; rest-any / rest-no-anchor; print-newline; port-nodigits-class / port-sign; etc.) y 19 NO equivalentes (F-05).

## Hallazgos
### F-05 · NARANJA · scripts/test/u11-egress-port-test.sh
Sobreviven mutantes no equivalentes de `llm_endpoint_host_port` (misma clase de F-01): 
1. Hosts reales sin cobertura (la prueba usa solo `h`, `H`, `::1`): si se quita `.`, `-`, `_` o `0-9` de `[A-Za-z0-9._-]+`, `https://api.deepseek.com`, `https://llm-gw.ns.svc:4000/v1` y `https://10.0.0.1:8443` pasan a rc=1 y eso rompería el control real; solo lo vería CA-3.
2. IPv6: con `\[[^]]+\]`, `https://[$(x)]` y `https://[;x]` saldrían con rc=0 imprimiendo `$(x) 443`; quitar `.`/minúsculas/mayúsculas de la clase hexadecimal no se detecta con `[::ffff:1.2.3.4]`, `[fe80::a]`, `[FE80::A]`; aceptar IPv6 sin corchetes (`https://::1`) tampoco.
3. Ceros a la izquierda / `10#`: sin él, `:08`, `:0080`, `:008080` dan rc=1 por octal y `:00065536` pasaría con `h 65536`; sin `10#` en el printf imprime `h 08`, `h 0080`, `h 0443`.
4. Esquema anclado: sin `^`, `xhttps://h`, `ftp://http://h` y `" https://h"` pasan.
5. Puerto vacío: con `([0-9]*)`, `https://h:` y `https://h:/x` pasan a `h 443`.
6. Fin del authority: `https://h?x`, `https://h:8443?x`, `https://h#f` y `https://a@b@h` (con `##*@` → `#*@` cambia) sin prueba.
Corrección propuesta (solo casos en la prueba): host con punto, guion, guion bajo y dígito; `[::ffff:1.2.3.4]`, `[fe80::a]`, `[FE80::A]` OK; `[;x]`, `[$(x)]`, `https://::1` error; `:08`→`h 8`, `:0080`→`h 80`, `:00065536` error; `xhttps://h`, `ftp://http://h` error; `https://h:` error; `https://h?x`, `https://h#f`, `https://a@b@h` OK.

### F-06 · AMARILLO · scripts/kind/lib.sh (`10#$port`)
Desbordamiento aritmético de 64 bits: `https://h:18446744073709551696` (2^64+80) da `h 80` rc=0 y `…617` da `h 1`. No explotable (la URL sale del pod y la pone el humano), pero contradice «fuera de 1-65535 → rc=1». Arreglo: rechazar más de 5 dígitos tras quitar los ceros, antes de la aritmética.

### F-07 · AMARILLO · bitacoras/U11-T05.md («CA-3 ronda 3»)
La sonda del puerto 80 se describe pero no se pega su comando literal; el bring-up se resume sin los comandos completos.

## Candidatas
Ninguna nueva. El alias de host sigue descartado.

VEREDICTO: NO-VERDE
