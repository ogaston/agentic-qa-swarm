package main

import rego.v1

test_isolation_local_sa_denied if {
	count(deny) > 0 with input as {"kind": "RoleBinding", "metadata": {"name": "b", "namespace": "aqs-test"}, "subjects": [{"kind": "ServiceAccount", "name": "cualquiera", "namespace": "aqs-test"}]}
}

test_isolation_control_plane_sa_allowed if {
	count(deny) == 0 with input as {"kind": "RoleBinding", "metadata": {"name": "b", "namespace": "aqs-test"}, "subjects": [{"kind": "ServiceAccount", "name": "go-reset", "namespace": "aqs-system"}]}
}
