package main

import rego.v1

# U4-T08: en aqs-system ninguna variable de entorno sensible lleva valor literal;
# debe venir de un Secret (valueFrom). Se deniega tambien value: "".

sufijos_sensibles := {"TOKEN", "SECRET", "PASSWORD", "PASSWD", "SECRET_KEY"}

pod_de(o) := o.spec.template.spec if o.kind in {"Deployment", "StatefulSet", "Job"}

pod_de(o) := o.spec.jobTemplate.spec.template.spec if o.kind == "CronJob"

# Todo contenedor del pod: normales, init y efimeros.
contenedores(pod) := array.concat(
	array.concat(object.get(pod, "containers", []), object.get(pod, "initContainers", [])),
	object.get(pod, "ephemeralContainers", []),
)

es_sensible(nombre) if {
	some s in sufijos_sensibles
	endswith(nombre, s)
}

deny contains msg if {
	input.metadata.namespace == "aqs-system"
	some c in contenedores(pod_de(input))
	some e in object.get(c, "env", [])
	es_sensible(e.name)
	"value" in object.keys(e)
	msg := sprintf("%s/%s: la variable %s es sensible y no puede llevar value literal; usa valueFrom.secretKeyRef", [input.kind, input.metadata.name, e.name])
}
