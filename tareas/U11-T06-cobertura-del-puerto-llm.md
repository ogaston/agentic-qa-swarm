# U11-T06 — Cobertura de la regla del puerto LLM (`llm_endpoint_host_port`)

**Unidad:** U11 — Demo con repositorio y modelo reales
**Historias que implementa:** US-M8.2 (el control de egreso debe ser fiable y comprobable).
**Depende de:** U11-T05 (PR #139; esta rama parte de `origin/tarea/U11-T05` mientras no se fusione: el alcance de CA-3 se mide contra esa rama). **Tope: 3 rondas**.

---

## Origen

U11-T05 arregló el control positivo de egreso del humo (`scripts/kind/kind-smoke.sh`, `llm_egress_acotado`) con la función `llm_endpoint_host_port` de `scripts/kind/lib.sh` y su prueba `scripts/test/u11-egress-port-test.sh`. En la ronda 3 el revisor reprodujo el control en un clúster real (3 × OK, puerto 80 bloqueado, `1.1.1.1:443` bloqueado, sin filtrar secretos, fuzz de 1523 URLs sin salidas fuera del patrón «host puerto»), pero un barrido de 95 mutaciones dejó **19 mutantes no equivalentes vivos** (`revisiones/U11-T05/ronda-3.md`, F-05), más un desbordamiento de 64 bits (F-06) y huecos de la bitácora (F-07). El humano cerró U11-T05 sin VERDE y trasladó esos hallazgos aquí (2026-10-26).

## Alcance

**Dentro** (una línea, concreta):

> Añadir a `scripts/test/u11-egress-port-test.sh` los casos que matan los 19 mutantes no equivalentes de F-05, rechazar en `llm_endpoint_host_port` los puertos de más de 5 dígitos (tras quitar los ceros a la izquierda) antes de la aritmética, y completar la bitácora de U11-T05 con los comandos literales.

Detalle:

1. **Casos nuevos (F-05)**, todos en la prueba existente:
   - hosts reales: `https://api.deepseek.com`, `https://llm-gw.ns.svc:4000/v1`, `https://10.0.0.1:8443`, `https://a_b-c.d9:443` → OK con el host y puerto esperados;
   - IPv6: `[::ffff:1.2.3.4]`, `[fe80::a]`, `[FE80::A]` → OK; `https://[;x]`, `https://[$(x)]`, `https://::1` → error;
   - ceros a la izquierda: `:08` → `h 8`, `:0080` → `h 80`, `:008080` → `h 8080`, `:00065536` → error;
   - ancla del esquema: `xhttps://h`, `ftp://http://h`, `" https://h"` → error;
   - puerto vacío: `https://h:`, `https://h:/x` → error;
   - fin del authority: `https://h?x` → `h 443`, `https://h:8443?x` → `h 8443`, `https://h#f` → `h 443`, `https://a@b@h` → `h 443`.
2. **Tope de dígitos (F-06):** `https://h:18446744073709551696` y `https://h:18446744073709551617` (2^64+80, 2^64+1) → error; `:00000080` → `h 80`; `:65535` → ok. Cambio mínimo en `lib.sh`: tras quitar los ceros a la izquierda, rechazar más de 5 dígitos antes de la comparación.
3. **Bitácora de U11-T05 (F-07):** pegar el comando literal de la sonda del puerto 80 y los comandos completos del bring-up.
4. **Guion de mutantes** `scripts/test/u11-egress-port-mutants.sh` (ver CA-2): reproducible, sin tocar `lib.sh`; la bitácora lista los 11 equivalentes del revisor con una línea de por qué lo son.

**Fuera** (lo que el codificador debe rechazar aunque lo vea roto):

- Cambiar la regla del puerto (https→443, http→80, explícito→ese), el control negativo `1.1.1.1:443`, la NetworkPolicy, `secrets.sh`, `deploy/`, agentes, servicios.
- Reescribir la función; solo se añade el tope de dígitos.
- El alias de host (`hostAliases`): descartado.

---

## Archivos de contexto

- `revisiones/U11-T05/ronda-3.md` (lista de mutantes y casos propuestos), `bitacoras/U11-T05.md`
- `scripts/kind/lib.sh` (`llm_endpoint_host_port`), `scripts/test/u11-egress-port-test.sh`, `scripts/kind/kind-smoke.sh` (solo lectura)

---

## Criterios de aceptación

- [ ] **CA-1** — La prueba cubre todos los casos listados y pasa.
  ```bash
  bash scripts/test/u11-egress-port-test.sh; echo "rc=$?"
  ```
  Esperado: termina en `OK` y `rc=0`, con al menos 25 + 30 casos (los previos y los nuevos del punto 1 y 2).

- [ ] **CA-2** — Los 19 mutantes no equivalentes de F-05 mueren y el desbordamiento está cerrado, **con un guion que se puede re-ejecutar**.
  ```bash
  bash scripts/test/u11-egress-port-mutants.sh; echo "rc=$?"
  bash -c '. scripts/kind/lib.sh; llm_endpoint_host_port https://h:18446744073709551696; echo "rc=$?"'
  ```
  Esperado: el guion nuevo `scripts/test/u11-egress-port-mutants.sh` aplica, sobre una **copia** temporal de `scripts/kind/lib.sh` (nunca sobre el original), cada uno de los 19 mutantes no equivalentes de F-05 (más los de puerto 0 y 65536 de F-01 y la regla vieja `u.port or 80`), ejecuta contra esa copia la prueba `u11-egress-port-test.sh` y comprueba que **FALLA**; imprime una línea `MUERTO <nombre>` por mutante y termina con `OK todos los mutantes mueren` y `rc=0`; si alguno sobrevive imprime `VIVO <nombre>` y sale distinto de 0. La prueba debe poder apuntar a la copia (p. ej. una variable de entorno `LIB_SH` que el guion de prueba use al hacer `source`, con valor por defecto `scripts/kind/lib.sh`). El segundo comando debe dar `rc=1` y ninguna salida.

- [ ] **CA-3** — Nada más cambió.
  ```bash
  bash -n scripts/kind/lib.sh scripts/kind/kind-smoke.sh && echo SINTAXIS_OK; git status --short | wc -l; b=$(git merge-base HEAD origin/tarea/U11-T05); git diff --name-only $b | grep -v -E '^(scripts/kind/lib\.sh|scripts/test/u11-egress-port-(test|mutants)\.sh|bitacoras/U11-T0[56]\.md|tareas/U11-T06-|unidades-y-tareas\.md|revisiones/U11-T06/)' | wc -l; git diff --name-only $b -- deploy services agents scripts/kind/kind-smoke.sh scripts/kind/secrets.sh | wc -l
  ```
  Esperado: `SINTAXIS_OK`, `0`, `0` y `0`. La verificación en un clúster real **no** se repite (la regla no cambia salvo el tope de dígitos, que el revisor comprueba sin clúster).

---

## Plan de pruebas

- **Rojo primero:** añadir los casos y mostrar cuáles fallan hoy (los del desbordamiento y, por construcción, ninguno de F-05 porque el código ya los cumple; lo que falta es que la prueba los mate al mutar). Pegar el barrido antes (supervivientes) y después (0 supervivientes).
- Par falla-pasa del tope de dígitos: sin él `…696` da `h 80`.

---

## Notas

- Archivos que se **modifican en su sitio**. Rama `tarea/U11-T06`, worktree `../agentic-qa-swarm-wt-U11-T06`, desde `origin/tarea/U11-T05` (apilada sobre el PR #139; si #139 ya está fusionada al abrir el PR de esta tarea, se rebasa/fusiona `origin/main` y el alcance se mide contra `origin/main`).
- Candidatas que surjan van a la bitácora.

---

_Creado con ❤️ por Luis Felipe Ariza Vesga._
