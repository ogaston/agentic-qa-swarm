package main

import rego.v1

ns_test := "aqs-test"

# Helpers para namespaces warm (aqs-test, aqs-w-*, o con etiqueta aqs.io/plane=warm)
es_warm_ns(ns) if ns == "aqs-test"
es_warm_ns(ns) if {
	is_string(ns)
	startswith(ns, "aqs-w-")
}

es_warm(obj) if {
	is_object(obj)
	ns := object.get(obj.metadata, "namespace", "")
	es_warm_ns(ns)
}
es_warm(obj) if {
	is_object(obj)
	labels := object.get(obj.metadata, "labels", {})
	object.get(labels, "aqs.io/plane", "") == "warm"
}
es_warm(obj) if {
	is_object(obj)
	obj.kind == "Namespace"
	es_warm_ns(object.get(obj.metadata, "name", ""))
}

cp_deployments := {"go-run-controller", "go-warm-manager", "go-reset"}

deny contains msg if {
	input.kind in {"ClusterRole", "ClusterRoleBinding"}
	input.metadata.name != "warm-provisioner"
	msg := sprintf("%s/%s: ClusterRole y ClusterRoleBinding estan prohibidos", [input.kind, input.metadata.name])
}

deny contains msg if {
	input.kind == "RoleBinding"
	es_warm(input)
	some s in input.subjects
	s.name == "aqs-runner"
	msg := sprintf("RoleBinding/%s: aqs-runner no puede tener RoleBindings en namespace warm", [input.metadata.name])
}

deny contains msg if {
	input.kind == "ServiceAccount"
	input.metadata.name == "aqs-runner"
	es_warm(input)
	object.get(input, "automountServiceAccountToken", true) != false
	msg := "ServiceAccount/aqs-runner: requiere automountServiceAccountToken: false"
}

deny contains msg if {
	input.kind == "NetworkPolicy"
	es_warm(input)
	some rule in array.concat(lista(object.get(input.spec, "egress", [])), lista(object.get(input.spec, "ingress", [])))
	some peer in array.concat(lista(object.get(rule, "to", [])), lista(object.get(rule, "from", [])))
	object.keys(peer)["ipBlock"]
	msg := sprintf("NetworkPolicy/%s: ipBlock prohibido en namespace warm", [input.metadata.name])
}

# null o no-arreglo cuenta como lista vacia
lista(x) := x if is_array(x)

lista(x) := [] if not is_array(x)

deny contains msg if {
	input.kind in {"Job", "CronJob"}
	es_warm(input)
	not pod_sa(input)
	msg := sprintf("%s/%s: falta serviceAccountName", [input.kind, input.metadata.name])
}

deny contains msg if {
	input.kind == "Deployment"
	input.metadata.name in cp_deployments
	not pod_sa(input)
	msg := sprintf("Deployment/%s: falta serviceAccountName", [input.metadata.name])
}

pod_sa(o) := o.spec.template.spec.serviceAccountName if o.kind == "Deployment"

pod_sa(o) := o.spec.template.spec.serviceAccountName if o.kind == "Job"

pod_sa(o) := o.spec.jobTemplate.spec.template.spec.serviceAccountName if o.kind == "CronJob"
