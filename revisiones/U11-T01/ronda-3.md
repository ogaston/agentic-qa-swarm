# Ronda 3 — U11-T01 (informe del revisor, persistido por el orquestador; resumen fiel del hand-back)

VEREDICTO: VERDE

Hash de la tarea ec96fc39… verificado. Worktree f1d38b9 limpio. Mutaciones en una copia del scratchpad. GOTOOLCHAIN=auto.

CA-1..CA-6 pasan con ejecución propia del revisor (policies.sh rc=0; `gofmt -l` vacío; archivos fuera de alcance = 0).

## Reproducción por identificador
- F-07 resuelto: con `replayed()=false` 0 avisos tras 4 Publish; con `replayed()=true` exactamente 1 WARN tras 8 intentos; sin SetReplayed 0 avisos.
- F-09(i) resuelto: `artifact` `{}`, sin kind, sin ref, vacíos o `null` → Publish devuelve error, 0 eventos, no se marca publicado; artefacto válido sí se publica.
- F-09(ii) resuelto: el golden falla con 7 mutaciones de NewRunConfirmed (quitar Artifact, cambiar Type/Version, quitar ConfirmedBy,
  Flows nil, desplazar OccurredAt, quitar NotificationID).
- F-08 resuelto. F-06 arbitrado (declarado en bitácora, «Para el PR»; Nota de la tarea corregida).
- Regresiones: SetReplayed cableado en cmd/ui-api antes de `go pub.Run`; con NATS usa un atomic.Bool tras completar el replay, con archivo `true`;
  el archivo nunca bloquea; `go test -race` en ui-api sin carreras.

## Hallazgos (ninguno bloquea)
- F-10 · AMARILLO · publish_test.go: la condición «no avisar antes del replay» no tiene prueba propia (quitarla no rompe la prueba del codificador).
- F-11 · AMARILLO · dos copias de `run-confirmed.json` (ui-api y go-run-controller) sin comprobación cruzada; hoy idénticas.
- F-12 · AMARILLO · publish.go:143-150: el mapa `warned` es en memoria; tras reiniciar el aviso se repite una vez por recibo antiguo (inocuo).

## Candidatas
- Comprobación en CI de que los dos fixtures son iguales (F-11).
- Reparar overlay kind y kind-e2e/kind-smoke/dashboard-smoke en U11-T02 (F-06).

VEREDICTO: VERDE
