package main

import rego.v1

# U4-T05 / U12: RBAC minimo. Sin wildcards, sin escalada ni acceso a secretos en ningun Role
# (con excepcion acotada: ClusterRole warm-provisioner solo con verbo create),
# y ningun RoleBinding (en cualquier namespace) para sujetos equivalentes a namespaces warm.

recursos_prohibidos := {"secrets", "pods/exec", "pods/attach", "pods/portforward", "serviceaccounts/token"}

verbos_prohibidos := {"escalate", "bind", "impersonate"}

campos_regla := ["apiGroups", "resources", "verbs"]

deny contains msg if {
	input.kind in {"Role", "ClusterRole"}
	some r in lista(object.get(input, "rules", []))
	some campo in campos_regla
	"*" in lista(object.get(r, campo, []))
	msg := sprintf("%s/%s: wildcard en %s prohibido", [input.kind, input.metadata.name, campo])
}

deny contains msg if {
	input.kind in {"Role", "ClusterRole"}
	some r in lista(object.get(input, "rules", []))
	some recurso in lista(object.get(r, "resources", []))
	recurso in recursos_prohibidos
	not excepcion_recurso_prohibido(input, r, recurso)
	msg := sprintf("%s/%s: el recurso %s esta prohibido (secretos/escalada)", [input.kind, input.metadata.name, recurso])
}

excepcion_recurso_prohibido(obj, r, recurso) if {
	obj.kind == "ClusterRole"
	obj.metadata.name == "warm-provisioner"
	recurso == "secrets"
	verbs := lista(object.get(r, "verbs", []))
	count(verbs) > 0
	every v in verbs {
		v == "create"
	}
}

deny contains msg if {
	input.kind in {"Role", "ClusterRole"}
	some r in lista(object.get(input, "rules", []))
	some verbo in lista(object.get(r, "verbs", []))
	verbo in verbos_prohibidos
	msg := sprintf("%s/%s: el verbo %s esta prohibido (escalada)", [input.kind, input.metadata.name, verbo])
}

# C-28: RoleBinding fuera de namespaces warm (dentro de namespaces warm ya lo cubre isolation.rego).
# Fuera de namespaces warm, un ServiceAccount sin namespace es del namespace del binding, no de warm.
deny contains msg if {
	input.kind == "RoleBinding"
	not es_warm(input)
	some s in lista(object.get(input, "subjects", []))
	sujeto_de_warm(s)
	msg := sprintf("RoleBinding/%s (%s): el sujeto %s/%s equivale a un ServiceAccount de un namespace warm", [input.metadata.name, object.get(input.metadata, "namespace", ""), s.kind, s.name])
}

sujeto_de_warm(s) if {
	s.kind == "ServiceAccount"
	es_warm_ns(s.namespace)
}

sujeto_de_warm(s) if {
	s.kind == "User"
	es_warm_user(s.name)
}

sujeto_de_warm(s) if {
	s.kind == "Group"
	s.name in grupos_prohibidos
}

sujeto_de_warm(s) if {
	s.kind == "Group"
	is_string(s.name)
	startswith(s.name, "system:serviceaccounts:aqs-w-")
}

sujeto_de_aqs_test(s) if sujeto_de_warm(s)
