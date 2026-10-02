package main

import rego.v1

cluster_kinds := {
	"Namespace",
	"ClusterRole",
	"ClusterRoleBinding",
	"CustomResourceDefinition",
	"PersistentVolume",
	"StorageClass",
	"PriorityClass",
	"IngressClass",
	"ValidatingWebhookConfiguration",
	"MutatingWebhookConfiguration",
	"APIService",
}

meta := m if {
	is_object(input)
	m := object.get(input, "metadata", {})
	is_object(m)
} else := {}

ns_valido if {
	ns := object.get(meta, "namespace", null)
	is_string(ns)
	trim_space(ns) != ""
}

ns_msg(kind, name) := sprintf("%s/%s: metadata.namespace es obligatorio (string no vacio)", [kind, name])

deny contains msg if {
	is_object(input)
	kind := object.get(input, "kind", "?")
	not kind in cluster_kinds
	not ns_valido
	msg := ns_msg(kind, object.get(meta, "name", "?"))
}
