F-06: Corregido en 5241e16: parches 01-no-confirm y 02-namespace-prefix regenerados contra el rules.go actual (git apply --check OK, cada uno rompe propiedades PBT con seed, -R los revierte); 03 y el de identity siguen aplicando; tocar esos archivos de T06 es consecuencia de este diff; PBT.md sigue correcto.
F-07: Corregido en 5241e16: casos «política extra válida» y «versión adelantada, mismo valor» en store_health_test.go; ambos mutantes de Healthy ahora fallan.
F-08: Fuera de alcance: tarea candidata propuesta (TTL para Healthy en /readyz, como el de la cadena de auditoría).
F-09: Corregido en 5241e16: README de go-identity separa las cabeceras de la API de la excepción de los endpoints operativos.
