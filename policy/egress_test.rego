package main

import rego.v1

np(rules) := {"apiVersion": "networking.k8s.io/v1", "kind": "NetworkPolicy", "metadata": {"name": "f", "namespace": "aqs-test"}, "spec": {"podSelector": {}, "policyTypes": ["Egress"], "egress": rules}}

ks := {"namespaceSelector": {"matchLabels": {"kubernetes.io/metadata.name": "kube-system"}}}

dns_ports := [{"port": 53, "protocol": "UDP"}, {"port": 53, "protocol": "TCP"}]

test_egress_intra_allowed if {
	count(deny) == 0 with input as np([{"to": [{"podSelector": {}}]}])
}

test_egress_dns_allowed if {
	count(deny) == 0 with input as np([{"to": [ks], "ports": dns_ports}])
}

test_egress_dns_with_podselector_allowed if {
	count(deny) == 0 with input as np([{"to": [object.union(ks, {"podSelector": {}})], "ports": [{"port": 53}]}])
}

test_egress_no_egress_rules_allowed if {
	count(deny) == 0 with input as {"kind": "NetworkPolicy", "metadata": {"name": "d", "namespace": "aqs-test"}, "spec": {"podSelector": {}, "policyTypes": ["Egress"]}}
}

test_egress_other_namespace_ignored if {
	count(deny) == 0 with input as object.union(np([{}]), {"metadata": {"name": "x", "namespace": "otro"}})
}

test_egress_open_denied if {
	count(deny) > 0 with input as np([{}])
}

test_egress_only_ports_denied if {
	count(deny) > 0 with input as np([{"ports": [{"port": 443}]}])
}

test_egress_empty_to_denied if {
	count(deny) > 0 with input as np([{"to": []}])
}

test_egress_all_namespaces_denied if {
	count(deny) > 0 with input as np([{"to": [{"namespaceSelector": {}}]}])
}

test_egress_other_namespace_denied if {
	count(deny) > 0 with input as np([{"to": [{"namespaceSelector": {"matchLabels": {"kubernetes.io/metadata.name": "aqs-system"}}}]}])
}

test_egress_matchexpressions_denied if {
	count(deny) > 0 with input as np([{"to": [{"namespaceSelector": {"matchExpressions": [{"key": "x", "operator": "Exists"}]}}]}])
}

test_egress_ipblock_denied if {
	count(deny) > 0 with input as np([{"to": [{"ipBlock": {"cidr": "10.0.0.0/8"}}]}])
}

test_egress_dns_443_denied if {
	count(deny) > 0 with input as np([{"to": [ks], "ports": [{"port": 443, "protocol": "TCP"}]}])
}

test_egress_dns_no_ports_denied if {
	count(deny) > 0 with input as np([{"to": [ks]}])
}

test_egress_intra_plus_ipblock_denied if {
	count(deny) > 0 with input as np([{"to": [{"podSelector": {}, "ipBlock": {"cidr": "0.0.0.0/0"}}]}])
}

test_egress_second_rule_bad_denied if {
	count(deny) > 0 with input as np([{"to": [{"podSelector": {}}]}, {}])
}
