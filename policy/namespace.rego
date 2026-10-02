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

ns_valido if {
	is_string(input.metadata.namespace)
	input.metadata.namespace != ""
}

deny contains msg if {
	not input.kind in cluster_kinds
	not ns_valido
	msg := sprintf("%s/%s: metadata.namespace es obligatorio (string no vacio)", [input.kind, object.get(input.metadata, "name", "?")])
}
