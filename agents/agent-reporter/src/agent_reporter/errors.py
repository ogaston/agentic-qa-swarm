class ReporterError(Exception):
    pass


class NoEvidence(ReporterError):
    """Sin evidencia valida: no se inventa un reporte (fail-closed)."""


class EvidenceUnavailable(ReporterError):
    pass


class InvalidEvidenceUri(EvidenceUnavailable):
    """El lector no pudo interpretar la URI: no cumple el contrato (se trata como NoEvidence, 422)."""


class BudgetExceeded(ReporterError):
    pass


class ReportRejected(ReporterError):
    def __init__(self, reason: str) -> None:
        super().__init__(reason)
        self.reason = reason


class StoreUnavailable(ReporterError):
    pass
