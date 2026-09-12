# Seguimiento de Aclaraciones — Requirements Analysis

Solo resta **1 ambigüedad**: en la clasificación 4 respondiste "A) Usar el pipeline de CI/CD
existente", pero la opción pedía indicar la herramienta concreta. Adjunto una segunda pregunta
para cerrar de una vez el mecanismo de despliegue GitOps (PRD §13 menciona "ArgoCD/Flux" sin
decidir).

---

## Pregunta A: Herramienta de CI/CD existente (RESILIENCY-04)

En la clasificación 4 elegiste "usar el pipeline de CI/CD existente". ¿Cuál es?

A) **GitHub Actions** — el pipeline del repo del producto corre sobre GitHub Actions (coherente con la premisa del producto, 100% GitHub) (Recomendado)

B) GitLab CI

C) Jenkins

D) Otro CI autohospedado (describe)

X) Otra (describe tras el tag [Answer])

[Answer]: A

---

## Pregunta B: Mecanismo de entrega GitOps del control plane (PRD §13 Módulo 8)

El PRD §13 deja el producto "desplegado y mantenido vía GitOps (ArgoCD/Flux)" sin elegir. ¿Qué encargado de entrega GitOps se especifica?

A) **ArgoCD** — ApplicationSet por entorno estable, sync manual a producción con promo entre entornos (Recomendado: modelo declarativo por entorno, popular en el ecosistema del curso)

B) Flux (Kustomize/Helm controllers, reconciliación automática)

C) No comprometer: documentar ambos y decidir en NFR/Infrastructure Design del primer despliegue

X) Otra (describe tras el tag [Answer])

[Answer]: B