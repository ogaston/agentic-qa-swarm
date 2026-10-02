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

test_deniega_namespace_solo_espacios if {
	count(deny) > 0 with input as {"kind": "ConfigMap", "metadata": {"name": "x", "namespace": " \n\t"}}
}

test_deniega_metadata_ausente if {
	ns_msg("ConfigMap", "?") in deny with input as {"kind": "ConfigMap"}
}

test_deniega_metadata_null if {
	ns_msg("ConfigMap", "?") in deny with input as {"kind": "ConfigMap", "metadata": null}
}

test_deniega_metadata_no_objeto if {
	ns_msg("ConfigMap", "?") in deny with input as {"kind": "ConfigMap", "metadata": "x"}
}

test_deniega_kind_ausente if {
	ns_msg("?", "x") in deny with input as {"metadata": {"name": "x"}}
}

test_permite_namespace_valido if {
	r := deny with input as {"kind": "ConfigMap", "metadata": {"name": "x", "namespace": "aqs-system"}}
	not ns_msg("ConfigMap", "x") in r
}

test_permite_kind_de_cluster if {
	r := deny with input as {"kind": "StorageClass", "metadata": {"name": "x"}}
	not ns_msg("StorageClass", "x") in r
}
