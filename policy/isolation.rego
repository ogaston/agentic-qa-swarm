package main

import rego.v1

# Ningun pod de aqs-test puede tener permisos sobre la API: se deniega todo
# RoleBinding en aqs-test cuyo sujeto sea un ServiceAccount del propio aqs-test.
deny contains msg if {
	input.kind == "RoleBinding"
	input.metadata.namespace == "aqs-test"
	some s in input.subjects
	s.kind == "ServiceAccount"
	s.namespace == "aqs-test"
	msg := sprintf("RoleBinding/%s: el ServiceAccount %s de aqs-test no puede tener permisos sobre la API", [input.metadata.name, s.name])
}
