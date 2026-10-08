# python:3.12-slim fijado por digest (indice multi-arch, resuelto el 2026-10-07).
FROM python:3.12-slim@sha256:2b4f19dae3a777dfc3b76730bda1e82e1f66ab2a2686fa93ca78edbfb4f04ffe
ENV PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1 PYTHONPATH=/app/src
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY src ./src
USER 65532:65532
EXPOSE 8080
CMD ["python", "-m", "agent_planner"]
