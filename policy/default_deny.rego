# Regla sobre el CONJUNTO del build. Solo dispara con --combine (input es entonces un
# arreglo de {path, contents}); sin --combine (CA-4 literal) no hace nada, por eso se
# ejecuta como comando aparte, documentado en la bitacora de U5-T06:
#   kustomize build deploy/flux/<env> | conftest test --policy policy --all-namespaces --combine -
package main

import rego.v1

deny contains msg if {
	is_array(input)
	not has_default_deny
	msg := "aqs-test: falta NetworkPolicy default-deny (podSelector {} con Ingress y Egress)"
}

has_default_deny if {
	some d in input
	o := d.contents
	o.kind == "NetworkPolicy"
	o.metadata.name == "default-deny"
	o.metadata.namespace == "aqs-test"
	count(o.spec.podSelector) == 0
	"Ingress" in o.spec.policyTypes
	"Egress" in o.spec.policyTypes
}
