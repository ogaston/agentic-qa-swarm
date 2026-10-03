package main

import rego.v1

# U4-T05: NetworkPolicy de aqs-system (solo ingress). Prohibido abrir el ingreso a todos
# (ipBlock, regla vacia, peer vacio, regla sin from) y exigido default-deny-ingress.

deny contains msg if {
	input.kind == "NetworkPolicy"
	input.metadata.namespace == "aqs-system"
	some regla in lista(object.get(input.spec, "ingress", []))
	some peer in lista(object.get(regla, "from", []))
	object.keys(peer)["ipBlock"]
	msg := sprintf("NetworkPolicy/%s: ipBlock prohibido en el ingress de aqs-system", [input.metadata.name])
}

# Regla sin from (incluye {}, from null y from []): permite a cualquier origen.
deny contains msg if {
	input.kind == "NetworkPolicy"
	input.metadata.namespace == "aqs-system"
	some regla in lista(object.get(input.spec, "ingress", []))
	count(lista(object.get(regla, "from", []))) == 0
	msg := sprintf("NetworkPolicy/%s: regla de ingress sin from abre el ingreso a todos", [input.metadata.name])
}

deny contains msg if {
	input.kind == "NetworkPolicy"
	input.metadata.namespace == "aqs-system"
	some regla in lista(object.get(input.spec, "ingress", []))
	some peer in lista(object.get(regla, "from", []))
	count(peer) == 0
	msg := sprintf("NetworkPolicy/%s: peer vacio ({}) en from abre el ingreso a todos", [input.metadata.name])
}

# Regla de conjunto: solo dispara con --combine.
deny contains msg if {
	is_array(input)
	not tiene_default_deny_ingress
	msg := "aqs-system: falta NetworkPolicy default-deny-ingress (podSelector {} con Ingress)"
}

tiene_default_deny_ingress if {
	some d in input
	o := d.contents
	o.kind == "NetworkPolicy"
	o.metadata.name == "default-deny-ingress"
	o.metadata.namespace == "aqs-system"
	count(o.spec.podSelector) == 0
	"Ingress" in o.spec.policyTypes
}
