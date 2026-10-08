class FlowValidationError(Exception):
    """Un flujo no paso una capa de validacion (plan, script o k6)."""

    def __init__(self, flow_id, capa, detalle=""):
        self.flow_id = flow_id
        self.capa = capa
        self.detalle = detalle
        super().__init__(f"flujo {flow_id!r} rechazado en capa {capa!r}: {detalle}")


class K6InspectError(Exception):
    """k6 inspect termino con codigo distinto de 0."""
