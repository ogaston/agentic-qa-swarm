package main

import rego.v1

# U4-T05: RBAC minimo. Sin wildcards, sin escalada ni acceso a secretos en ningun Role,
# y ningun RoleBinding (en cualquier namespace) para sujetos equivalentes a aqs-test.

recursos_prohibidos := {"secrets", "pods/exec", "pods/attach", "pods/portforward", "serviceaccounts/token"}

verbos_prohibidos := {"escalate", "bind", "impersonate"}

campos_regla := ["apiGroups", "resources", "verbs"]

deny contains msg if {
	input.kind == "Role"
	some r in lista(object.get(input, "rules", []))
	some campo in campos_regla
	"*" in lista(object.get(r, campo, []))
	msg := sprintf("Role/%s: wildcard en %s prohibido", [input.metadata.name, campo])
}

deny contains msg if {
	input.kind == "Role"
	some r in lista(object.get(input, "rules", []))
	some recurso in lista(object.get(r, "resources", []))
	recurso in recursos_prohibidos
	msg := sprintf("Role/%s: el recurso %s esta prohibido (secretos/escalada)", [input.metadata.name, recurso])
}

deny contains msg if {
	input.kind == "Role"
	some r in lista(object.get(input, "rules", []))
	some verbo in lista(object.get(r, "verbs", []))
	verbo in verbos_prohibidos
	msg := sprintf("Role/%s: el verbo %s esta prohibido (escalada)", [input.metadata.name, verbo])
}

# C-28: RoleBinding fuera de aqs-test (dentro de aqs-test ya lo cubre isolation.rego).
# Fuera de aqs-test, un ServiceAccount sin namespace es del namespace del binding, no de aqs-test.
deny contains msg if {
	input.kind == "RoleBinding"
	object.get(input.metadata, "namespace", "") != ns_test
	some s in lista(object.get(input, "subjects", []))
	sujeto_de_aqs_test(s)
	msg := sprintf("RoleBinding/%s (%s): el sujeto %s/%s equivale a un ServiceAccount de aqs-test", [input.metadata.name, object.get(input.metadata, "namespace", ""), s.kind, s.name])
}

sujeto_de_aqs_test(s) if {
	s.kind == "ServiceAccount"
	s.namespace == ns_test
}

sujeto_de_aqs_test(s) if {
	s.kind == "User"
	startswith(s.name, sa_prefix)
}

sujeto_de_aqs_test(s) if {
	s.kind == "Group"
	s.name in grupos_prohibidos
}
