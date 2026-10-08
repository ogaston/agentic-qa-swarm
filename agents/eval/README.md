# agents/eval

Evaluacion offline (U3-T05): ejecuta `agent-planner` y `agent-reporter` como biblioteca sobre los artefactos de
`agents/dataset` con su `FakeLLM`, calcula las metricas, escribe `out/report.json` y falla si no se cumplen los umbrales.

> **Alcance, dicho sin rodeos (C-85).** Con `FakeLLM` las respuestas son canned. El informe prueba que el pipeline,
> las validaciones y la redaccion funcionan y que una regresion se detecta. **No** mide la calidad de un modelo real:
> afirmar "precision > 80 %" del producto exige medir con un modelo real (U3-T07 y una tarea candidata).
> El informe lo declara con `llm: "fake"` y `valid_for: "pipeline"` (literales obligatorios en `report.schema.json`).

## Uso

```
python -m venv .venv && .venv/bin/pip install -r requirements-dev.txt
.venv/bin/python -m agent_eval run [--dataset ../dataset] [--out out]      # rc 0 umbrales OK, 1 FALLA, 2 dataset invalido
.venv/bin/python -m agent_eval add-regression --artifact <id> --finding <finding_id> --label sin-hallazgos|inconcluso [--dataset ../dataset]
```

Sin `Dockerfile`: no es un servicio. No usa red (guarda propia que bloquea y cuenta conexiones no locales).

## Metricas (definiciones fijas)

- **factualidad** = artefactos `bug-sembrado` con algun hallazgo `(invariant, method, path)` igual a la causa raiz esperada / artefactos `bug-sembrado`.
- **precision** = artefactos etiquetados (`golden`, `bug-sembrado`, `trampa-esquema` y `regresion-fp`) con veredicto igual al esperado y, si es `bug`, causa raiz esperada entre los hallazgos / etiquetados. Hoy son 10 (con una regresion, 11).
- **ruido** = hallazgos falsos positivos / hallazgos emitidos. Falso positivo: cualquier hallazgo en un artefacto cuyo veredicto esperado no es `bug`, o con causa raiz distinta de la esperada. Sin hallazgos, `0.0`.
- **adherencia** = comprobaciones superadas / comprobaciones aplicables (A1-A7). Las reglas que imponen U2/U4 (reset verificado, aislamiento del namespace, tope de intentos) no se evaluan aqui.
  A1 todo paso cita un endpoint observado; A2 respeta `max_flows`/`max_steps` y sin rutas absolutas, `..` ni host; A3 `no-arranca` falla cerrado (`NoSurface` y `NoEvidence`); A4 todo hallazgo cita solo URIs recibidas; A5 ni prompts ni reportes contienen un patron de secreto (se usa el redactor del reporter como detector: un texto ya redactado no debe cambiar); A6 cero conexiones no locales; A7 el prompt del planner es exactamente la plantilla mas la superficie canonica.

## Umbrales (`thresholds.py`, citados en el informe)

| metrica | umbral | fuente |
|---|---|---|
| precision | `> 0.80` | specs/prd.md §10 |
| ruido | `< 0.20` (estricto) | specs/prd.md §10 |
| adherencia | `== 1.0` | propuesto aqui (C-89), solo reglas que dependen de U3; no esta en el PRD |
| factualidad | `> 0.80` | propuesto aqui (misma barra que precision; PRD §11 pide "coincidir con la real") |

Se comparan como fracciones exactas. Aflojar un umbral o una regla no es una forma valida de arreglar una metrica.

## Bucle de regresion

`add-regression` copia superficie y evidencia de un artefacto a `<dataset>/regressions/<id>-fp-<n>/`
(`meta.kind = "regresion-fp"`), fija `expected.json` con la etiqueta dada y **conserva** `reporter.response.json`
(la respuesta que produjo el falso positivo): la regresion falla (ruido sube, precision baja) hasta que se corrija el
agente o el dato. `run` incluye `regressions/*` (`dataset.regressions`). `--finding` se registra en `meta.json`
(`source_finding`); si no figura en la respuesta canned, se avisa por stderr.
