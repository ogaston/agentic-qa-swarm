package main

import rego.v1

# Ningun pod de namespace warm puede tener permisos sobre la API: se deniega todo
# RoleBinding en namespace warm cuyo sujeto sea un ServiceAccount del propio warm,
# o un sujeto que Kubernetes trate como equivalente.

sa_prefix := "system:serviceaccount:aqs-test:"

grupos_prohibidos := {
	"system:serviceaccounts",
	"system:serviceaccounts:aqs-test",
	"system:authenticated",
	"system:unauthenticated",
}

# Helpers para usuarios sinteticos
es_warm_user(name) if {
	is_string(name)
	startswith(name, "system:serviceaccount:aqs-test:")
}
es_warm_user(name) if {
	is_string(name)
	startswith(name, "system:serviceaccount:aqs-w-")
}

# ServiceAccount de namespace warm; sin namespace el authorizer usa el del RoleBinding.
sujeto_warm(s, target_ns) if {
	s.kind == "ServiceAccount"
	ns_warm_sa(object.get(s, "namespace", ""), target_ns)
}

# Ausente, vacio, null o no-string cuentan como ausente.
ns_warm_sa(ns, target_ns) if not is_string(ns)

ns_warm_sa(ns, target_ns) if ns in {"", target_ns}

ns_warm_sa(ns, _) if es_warm_ns(ns)

# User con el nombre sintetico de un SA de un warm namespace.
sujeto_warm(s, _) if {
	s.kind == "User"
	es_warm_user(s.name)
}

sujeto_warm(s, _) if {
	s.kind == "Group"
	s.name in grupos_prohibidos
}

sujeto_warm(s, _) if {
	s.kind == "Group"
	is_string(s.name)
	startswith(s.name, "system:serviceaccounts:aqs-w-")
}

# Alias para compatibilidad
sujeto_aqs_test(s) if sujeto_warm(s, "aqs-test")

deny contains msg if {
	input.kind == "RoleBinding"
	es_warm(input)
	target_ns := object.get(input.metadata, "namespace", "")
	some s in lista(object.get(input, "subjects", []))
	sujeto_warm(s, target_ns)
	msg := sprintf("RoleBinding/%s: el sujeto %s/%s equivale a un ServiceAccount de un namespace warm y no puede tener permisos sobre la API", [input.metadata.name, s.kind, s.name])
}
