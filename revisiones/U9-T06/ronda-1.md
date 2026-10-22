# Ronda 1 — U9-T06

VEREDICTO: VERDE

Sha revisado 46c33a9. El revisor corrió CA-1 a CA-5 desde `web/dashboard/`: CA-1 a CA-4 pasan (170 de 170 pruebas; `style={{` = 0; `check:dist-urls` da 0/salida 0 y 1/salida 1 con la URL sembrada). CA-5 da 1 en el filtro de `git diff --name-only`: es `unidades-y-tareas.md`, que añadió el commit de despacho 5938273 del orquestador, no el codificador (los commits 9d112de y 46c33a9 tocan solo rutas dentro del alcance). Arbitraje del orquestador: se acepta; la fila `U9-T6` se marca `[x]` al fusionar.

## Hallazgos
### F-01 · AMARILLO · `web/dashboard/scripts/check-dist-urls.sh:25-27`
Falsos negativos menores: `<script src="//cdn.x/y.js">`, `HREF="HTTPS://..."` (sin `-i`) y `href="http://w3.org.evil.com/x"` (`grep -v 'w3\.org'` descarta por subcadena). No bloquea; los dos primeros no los exige la tarea y el tercero es hipotético en un build de Vite.

## Candidata
Endurecer `check-dist-urls.sh` (insensible a mayúsculas, `//host`, exclusión de `w3.org` por host exacto). Baja prioridad.

VEREDICTO: VERDE
AMARILLO|web/dashboard/scripts/check-dist-urls.sh:25-27|Falsos negativos menores (mayúsculas, //host, w3.org como subcadena)
