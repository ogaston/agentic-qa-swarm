# U4-T08 — respuesta a la ronda 1

F-01: Corregido en SHA_PLACEHOLDER: sección 6 reescrita con un único bloque (`encrypt ...` sin `sops` como generate.sh:108, un solo `d` y un solo `trap`); ensayado con docker falso, rojo y verde en la bitácora.
F-02: Corregido en SHA_PLACEHOLDER: `secretrefs.rego` recorre `containers`, `initContainers` y `ephemeralContainers`; 7 pruebas nuevas y negativos de initContainer/ephemeral rc=1 (antes rc=0).
F-03: Corregido en SHA_PLACEHOLDER: 6.4 documenta `kubectl describe pod` y los estados `CreateContainerConfigError` y `ContainerCreating`/`FailedMount`; advertencia corregida.
F-04: Corregido en SHA_PLACEHOLDER: 6.1 indica `cd services/go-identity && go run ./cmd/go-identity hash-password` (o la imagen).
