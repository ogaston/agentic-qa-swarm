package main

import rego.v1

np_sys(ingress) := {"kind": "NetworkPolicy", "metadata": {"name": "n", "namespace": "aqs-system"}, "spec": {"podSelector": {}, "policyTypes": ["Ingress"], "ingress": ingress}}

test_ingress_ipblock_denied if {
	count(deny) > 0 with input as np_sys([{"from": [{"ipBlock": {"cidr": "0.0.0.0/0"}}]}])
}

test_ingress_mixto_con_ipblock_denied if {
	count(deny) > 0 with input as np_sys([{"from": [{"podSelector": {}}, {"ipBlock": {"cidr": "10.0.0.0/8"}}]}])
}

test_ingress_regla_vacia_denied if {
	count(deny) > 0 with input as np_sys([{}])
}

test_ingress_from_null_denied if {
	count(deny) > 0 with input as np_sys([{"from": null, "ports": [{"port": 8080}]}])
}

test_ingress_peer_vacio_denied if {
	count(deny) > 0 with input as np_sys([{"from": [{}]}])
}

test_ingress_valido_sin_ports_allowed if {
	count(deny) == 0 with input as np_sys([{"from": [{"podSelector": {}}]}])
}

test_ingress_null_allowed if {
	count(deny) == 0 with input as np_sys(null)
}

test_ingress_ipblock_en_otro_namespace_no_aplica if {
	count(deny) == 0 with input as object.union(np_sys([{"from": [{"ipBlock": {"cidr": "0.0.0.0/0"}}]}]), {"metadata": {"namespace": "otro"}})
}

ddi(ns, types) := {"contents": {"kind": "NetworkPolicy", "metadata": {"name": "default-deny-ingress", "namespace": ns}, "spec": {"podSelector": {}, "policyTypes": types}}}

dd_test := {"contents": {"kind": "NetworkPolicy", "metadata": {"name": "default-deny", "namespace": "aqs-test"}, "spec": {"podSelector": {}, "policyTypes": ["Ingress", "Egress"]}}}

test_combine_presente_allowed if {
	count(deny) == 0 with input as [ddi("aqs-system", ["Ingress"]), dd_test]
}

test_combine_ausente_denied if {
	count(deny) > 0 with input as [{"contents": {"kind": "Namespace", "metadata": {"name": "x"}}}]
}

test_combine_solo_egress_denied if {
	count(deny) > 0 with input as [ddi("aqs-system", ["Egress"])]
}

test_combine_otro_namespace_denied if {
	count(deny) > 0 with input as [ddi("aqs-test", ["Ingress"])]
}
