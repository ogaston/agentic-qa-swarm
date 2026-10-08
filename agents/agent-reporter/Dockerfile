FROM python:3.12-slim@sha256:2b4f19dae3a777dfc3b76730bda1e82e1f66ab2a2686fa93ca78edbfb4f04ffe
# Sin dependencias de ejecucion de terceros: solo biblioteca estandar.
WORKDIR /app
COPY src/ /app/src/
ENV PYTHONPATH=/app/src PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1
USER 65532:65532
EXPOSE 8080
CMD ["python", "-m", "agent_reporter.server"]
