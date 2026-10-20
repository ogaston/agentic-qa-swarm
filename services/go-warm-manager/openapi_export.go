package warmmanager

// OpenAPIEndpoints devuelve los endpoints (con request_body cuando el OpenAPI lo declara en
// application/json) del documento dado. Es la misma inferencia que usa InferSurface; la expone
// para que pruebas de integracion de otros modulos (aqs-runner) partan de la superficie real.
func OpenAPIEndpoints(doc []byte) ([]Endpoint, bool) {
	return parseOpenAPI(doc)
}
