package main

import rego.v1

# U4-T05: identidad y aislamiento de pods. C-21 (serviceAccountName en los siete
# Deployments de aqs-system), C-22 (automount en aqs-test) y red del nodo en aqs-test.

todos_cp := {"ui-api", "go-intake", "go-governance", "go-identity", "go-run-controller", "go-warm-manager", "go-reset"}

sin_api := {"ui-api", "go-intake", "go-governance", "go-identity"}

plantilla(o) := o.spec.template.spec if o.kind in {"Deployment", "StatefulSet", "Job"}

plantilla(o) := o.spec.jobTemplate.spec.template.spec if o.kind == "CronJob"

es_control_plane(o) if {
	o.kind == "Deployment"
	o.metadata.namespace == "aqs-system"
	o.metadata.name in todos_cp
}

sa_valido(spec) if {
	n := spec.serviceAccountName
	is_string(n)
	n != ""
}

deny contains msg if {
	es_control_plane(input)
	not sa_valido(plantilla(input))
	msg := sprintf("Deployment/%s: serviceAccountName ausente, vacio o no-string", [input.metadata.name])
}

deny contains msg if {
	es_control_plane(input)
	input.metadata.name in sin_api
	object.get(plantilla(input), "automountServiceAccountToken", true) != false
	msg := sprintf("Deployment/%s: requiere automountServiceAccountToken: false en el pod", [input.metadata.name])
}

en_aqs_test(o) if {
	o.metadata.namespace == ns_test
	o.kind in {"Deployment", "StatefulSet", "Job", "CronJob"}
}

deny contains msg if {
	en_aqs_test(input)
	object.get(plantilla(input), "automountServiceAccountToken", false) in {true, "true"}
	msg := sprintf("%s/%s: automountServiceAccountToken: true prohibido en aqs-test", [input.kind, input.metadata.name])
}

# Cualquier valor distinto de ausente/null/false cuenta como activado (incluido "true" como string).
deny contains msg if {
	en_aqs_test(input)
	some campo in {"hostNetwork", "hostPID", "hostIPC"}
	v := object.get(plantilla(input), campo, false)
	v != false
	v != null
	msg := sprintf("%s/%s: %s prohibido en aqs-test", [input.kind, input.metadata.name, campo])
}
