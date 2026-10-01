# Egress en aqs-test (U5-T10, C-19): solo se aceptan reglas (a) intra-namespace o (b) DNS a kube-system:53.
package main

import rego.v1

deny contains msg if {
	input.kind == "NetworkPolicy"
	input.metadata.namespace == "aqs-test"
	some i, rule in input.spec.egress
	not egress_rule_ok(rule)
	msg := sprintf("aqs-test: NetworkPolicy %s, egress[%d] no es intra-namespace ni DNS a kube-system:53", [input.metadata.name, i])
}

egress_rule_ok(rule) if egress_intra(rule)

egress_rule_ok(rule) if egress_dns(rule)

# (a) todos los peers son solo podSelector
egress_intra(rule) if {
	count(rule.to) > 0
	every peer in rule.to {
		peer_intra(peer)
	}
}

peer_intra(peer) if {
	is_object(peer.podSelector)
	count(object.keys(peer)) == 1
}

# (b) todos los peers son kube-system (matchLabels exacto) y los puertos son solo 53 UDP/TCP
egress_dns(rule) if {
	count(rule.to) > 0
	every peer in rule.to {
		peer_dns(peer)
	}
	count(rule.ports) > 0
	every p in rule.ports {
		dns_port(p)
	}
}

peer_dns(peer) if {
	not peer.ipBlock
	peer.namespaceSelector == {"matchLabels": {"kubernetes.io/metadata.name": "kube-system"}}
}

dns_port(p) if {
	p.port == 53
	object.get(p, "protocol", "TCP") in {"UDP", "TCP"}
	not p.endPort
}
