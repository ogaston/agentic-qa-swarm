# Límite de autonomía del agente

## Overview
Estas reglas son restricciones bloqueantes en todas las fases de AI-DLC. No son
recomendaciones. Cada etapa DEBE verificarlas antes de presentar su mensaje de
finalización.

### Default Enforcement
Todas las reglas de este documento son **bloqueantes**. Si no se cumple un criterio de
verificación, es un hallazgo bloqueante: la etapa solo puede ofrecer «Request Changes».

---

## Rule AUTONOMIA-01: Ninguna tarea aplica cambios a infraestructura sin aprobación

**Rule**: Ninguna unidad de trabajo puede contener una tarea que aplique cambios a un
clúster de Kubernetes, a la nube o a cualquier entorno compartido sin una aprobación
humana registrada. El destino de la cadena del agente es un pull request con evidencia
adjunta, nunca un `kubectl apply` autónomo.

**Verification**:
- Ningún plan de tareas contiene un paso que ejecute `kubectl apply`, `terraform apply`,
  `helm install` o equivalente sin un paso previo de aprobación humana
- Toda tarea que toque un entorno compartido produce un artefacto revisable, no un cambio
  directo

---

## Rule AUTONOMIA-02: Todo criterio de aceptación se verifica con un comando

**Rule**: Cada tarea de cada unidad DEBE tener un criterio de aceptación comprobable
ejecutando un comando. Las formulaciones de opinión no son criterios de aceptación.

**Verification**:
- Ningún criterio de aceptación usa formulaciones como «funciona correctamente»,
  «es usable» o «tiene buen rendimiento» sin un comando o umbral medible
- Cada tarea nombra el comando, la prueba o la comprobación que demuestra que terminó
