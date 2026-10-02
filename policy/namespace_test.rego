package main

import rego.v1

test_deniega_sin_namespace if {
	count(deny) > 0 with input as {"kind": "ConfigMap", "metadata": {"name": "x"}}
}

test_deniega_namespace_null if {
	count(deny) > 0 with input as {"kind": "ConfigMap", "metadata": {"name": "x", "namespace": null}}
}

test_deniega_namespace_vacio if {
	count(deny) > 0 with input as {"kind": "CronJob", "metadata": {"name": "x", "namespace": ""}}
}

test_deniega_namespace_no_string if {
	count(deny) > 0 with input as {"kind": "ConfigMap", "metadata": {"name": "x", "namespace": 5}}
}

test_permite_namespace_valido if {
	r := deny with input as {"kind": "ConfigMap", "metadata": {"name": "x", "namespace": "aqs-system"}}
	not "ConfigMap/x: metadata.namespace es obligatorio (string no vacio)" in r
}

test_permite_kind_de_cluster if {
	r := deny with input as {"kind": "StorageClass", "metadata": {"name": "x"}}
	not "StorageClass/x: metadata.namespace es obligatorio (string no vacio)" in r
}
