F-01: Corregido en 1469daf: la existencia del workflow se exige en todo destino salvo resetting/reporting/done/failed (también warm_ready e inferring); pruebas por destino (TestWorkflowExistenceByDestination) y mutante que revierte el cambio falla; documentado en authz/doc.go.
F-02: No bloquea: documentado en authz/doc.go que workflow_allowed en warm_ready es autodeclarado y que U2-T02 debe reportarlo antes de la selección de workflow.
F-03: Fuera de alcance: tarea candidata propuesta (hash del valor de la política en policy.set y versión+hash en cada gate).
F-04: Fuera de alcance: tarea candidata propuesta (decodificación estricta de claves, duplicados y mensajes de 422 sin texto interno); el llamante es un servicio de confianza.
F-05: Fuera de alcance: tarea candidata propuesta (lista blanca de GOVERNANCE_ENV y de GOVERNANCE_TEST_NAMESPACE en lugar de lista negra); la regla actual es literal a la tarea.
F-06: Corregido en 1469daf: TestAppendSyncsEveryEntry cuenta Sync por entrada y TestSyncFailureFailsAppendAndPoisons cubre fsync fallido; el mutante sin Sync falla ambas.
