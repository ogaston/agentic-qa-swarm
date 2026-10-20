// Comando de apoyo a las pruebas de integracion (U8-T09a CA-4): lee un OpenAPI por stdin y escribe
// en stdout los endpoints que infiere parseOpenAPI (con request_body cuando lo declara).
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	wm "github.com/ogaston/agentic-qa-swarm/services/go-warm-manager"
)

func main() {
	doc, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	eps, ok := wm.OpenAPIEndpoints(doc)
	if !ok {
		fmt.Fprintln(os.Stderr, "OpenAPI no interpretable")
		os.Exit(1)
	}
	if err := json.NewEncoder(os.Stdout).Encode(eps); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
