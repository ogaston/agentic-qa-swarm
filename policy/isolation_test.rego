package main

import rego.v1

test_isolation_local_sa_denied if {
	count(deny) > 0 with input as {"kind": "RoleBinding", "metadata": {"name": "b", "namespace": "aqs-test"}, "subjects": [{"kind": "ServiceAccount", "name": "cualquiera", "namespace": "aqs-test"}]}
}

test_isolation_control_plane_sa_allowed if {
	count(deny) == 0 with input as {"kind": "RoleBinding", "metadata": {"name": "b", "namespace": "aqs-test"}, "subjects": [{"kind": "ServiceAccount", "name": "go-reset", "namespace": "aqs-system"}]}
}

rb(s) := {"kind": "RoleBinding", "metadata": {"name": "b", "namespace": "aqs-test"}, "subjects": [s]}

test_isolation_sa_without_namespace_denied if {
	count(deny) > 0 with input as rb({"kind": "ServiceAccount", "name": "x"})
}

test_isolation_sa_empty_namespace_denied if {
	count(deny) > 0 with input as rb({"kind": "ServiceAccount", "name": "x", "namespace": ""})
}

test_isolation_user_synthetic_sa_denied if {
	count(deny) > 0 with input as rb({"kind": "User", "name": "system:serviceaccount:aqs-test:x"})
}

test_isolation_group_serviceaccounts_denied if {
	count(deny) > 0 with input as rb({"kind": "Group", "name": "system:serviceaccounts"})
}

test_isolation_group_serviceaccounts_aqs_test_denied if {
	count(deny) > 0 with input as rb({"kind": "Group", "name": "system:serviceaccounts:aqs-test"})
}

test_isolation_group_authenticated_denied if {
	count(deny) > 0 with input as rb({"kind": "Group", "name": "system:authenticated"})
}

test_isolation_group_unauthenticated_denied if {
	count(deny) > 0 with input as rb({"kind": "Group", "name": "system:unauthenticated"})
}

test_isolation_user_other_allowed if {
	count(deny) == 0 with input as rb({"kind": "User", "name": "system:serviceaccount:aqs-system:go-reset"})
}
