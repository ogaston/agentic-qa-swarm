# Aprobación de recuperación local del Paso 0

La API de kind responde y los namespaces existen. El rollout de `warm-app` está bloqueado por una imagen inexistente. La corrección del diagnóstico está lista y pasa 8 pruebas aisladas.

## Pregunta 1

¿Apruebas restaurar la imagen de referencia de `warm-app` en `kind-aqs/aqs-test` y repetir el setup local?

A) Aprobar la recuperación: ejecutar los tres comandos siguientes. Incluye los port-forwards, el túnel público de go-intake y el pod temporal de humo que administra `00-setup.sh`. No incluye enviar eventos GitHub ni confirmar una corrida de QA.

B) Request Changes: indicar los cambios deseados antes de modificar el clúster.

[Answer]: A — aprobado por el usuario mediante «hazlo», registrado en auditoría el 2026-10-28T14:33:36Z.

```bash
kubectl --context kind-aqs -n aqs-test set image deployment/warm-app warm-app=ghcr.io/ogaston/demo-target-app-qa-swarm:b97a719199d5ccbc2ff24f9cfc408ec7ee31114d
kubectl --context kind-aqs -n aqs-test rollout status deployment/warm-app --timeout=180s
bash scripts/demo/00-setup.sh
```

Motivo de la aprobación: `.aidlc-rule-details/extensions/autonomia/limite/limite-autonomia.md`, AUTONOMIA-01: «Ninguna unidad de trabajo puede contener una tarea que aplique cambios a un clúster de Kubernetes, a la nube o a cualquier entorno compartido sin una aprobación humana registrada».

Evidencia y alcance: `aidlc-docs/construction/demo-setup/code/verification.md`.
