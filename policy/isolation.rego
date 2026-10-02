package main

import rego.v1

# Ningun pod de aqs-test puede tener permisos sobre la API: se deniega todo
# RoleBinding en aqs-test cuyo sujeto sea un ServiceAccount del propio aqs-test,
# o un sujeto que Kubernetes trate como equivalente.

sa_prefix := "system:serviceaccount:aqs-test:"

grupos_prohibidos := {
	"system:serviceaccounts",
	"system:serviceaccounts:aqs-test",
	"system:authenticated",
	"system:unauthenticated",
}

# ServiceAccount de aqs-test; sin namespace el authorizer usa el del RoleBinding.
sujeto_aqs_test(s) if {
	s.kind == "ServiceAccount"
	ns_ausente_o_local(object.get(s, "namespace", ""))
}

# Ausente, vacio, null o no-string cuentan como ausente.
ns_ausente_o_local(ns) if not is_string(ns)

ns_ausente_o_local(ns) if ns in {"", "aqs-test"}

# User con el nombre sintetico de un SA de aqs-test.
sujeto_aqs_test(s) if {
	s.kind == "User"
	startswith(s.name, sa_prefix)
}

sujeto_aqs_test(s) if {
	s.kind == "Group"
	s.name in grupos_prohibidos
}

deny contains msg if {
	input.kind == "RoleBinding"
	input.metadata.namespace == "aqs-test"
	some s in input.subjects
	sujeto_aqs_test(s)
	msg := sprintf("RoleBinding/%s: el sujeto %s/%s equivale a un ServiceAccount de aqs-test y no puede tener permisos sobre la API", [input.metadata.name, s.kind, s.name])
}
