package main

import rego.v1

role(rules) := {"kind": "Role", "metadata": {"name": "r", "namespace": "aqs-test"}, "rules": rules}

rbg(ns, s) := {"kind": "RoleBinding", "metadata": {"name": "b", "namespace": ns}, "subjects": [s]}

test_role_wildcard_verbs_denied if {
	count(deny) > 0 with input as role([{"apiGroups": [""], "resources": ["pods"], "verbs": ["*"]}])
}

test_role_wildcard_resources_denied if {
	count(deny) > 0 with input as role([{"apiGroups": [""], "resources": ["*"], "verbs": ["get"]}])
}

test_role_wildcard_apigroups_denied if {
	count(deny) > 0 with input as role([{"apiGroups": ["*"], "resources": ["pods"], "verbs": ["get"]}])
}

test_role_sin_wildcard_allowed if {
	count(deny) == 0 with input as role([{"apiGroups": [""], "resources": ["pods", "pods/log"], "verbs": ["get", "list"]}])
}

test_role_secrets_with_others_denied if {
	count(deny) > 0 with input as role([{"apiGroups": [""], "resources": ["configmaps", "secrets"], "verbs": ["get"]}])
}

test_role_pods_exec_get_denied if {
	count(deny) > 0 with input as role([{"apiGroups": [""], "resources": ["pods/exec"], "verbs": ["get"]}])
}

test_role_attach_portforward_token_denied if {
	count(deny) > 0 with input as role([{"apiGroups": [""], "resources": ["pods/attach"], "verbs": ["create"]}])
	count(deny) > 0 with input as role([{"apiGroups": [""], "resources": ["pods/portforward"], "verbs": ["create"]}])
	count(deny) > 0 with input as role([{"apiGroups": [""], "resources": ["serviceaccounts/token"], "verbs": ["create"]}])
}

test_role_escalate_bind_impersonate_denied if {
	count(deny) > 0 with input as role([{"apiGroups": [""], "resources": ["roles"], "verbs": ["escalate"]}])
	count(deny) > 0 with input as role([{"apiGroups": [""], "resources": ["roles"], "verbs": ["bind"]}])
	count(deny) > 0 with input as role([{"apiGroups": [""], "resources": ["users"], "verbs": ["impersonate"]}])
}

test_role_sin_rules_allowed if {
	count(deny) == 0 with input as {"kind": "Role", "metadata": {"name": "r", "namespace": "aqs-test"}}
}

test_binding_ajeno_sa_aqs_test_denied if {
	count(deny) > 0 with input as rbg("aqs-system", {"kind": "ServiceAccount", "name": "aqs-runner", "namespace": "aqs-test"})
}

test_binding_ajeno_user_sintetico_denied if {
	count(deny) > 0 with input as rbg("default", {"kind": "User", "name": "system:serviceaccount:aqs-test:x"})
}

test_binding_ajeno_group_denied if {
	count(deny) > 0 with input as rbg("default", {"kind": "Group", "name": "system:serviceaccounts"})
}

test_binding_ajeno_sa_local_sin_namespace_allowed if {
	count(deny) == 0 with input as rbg("aqs-system", {"kind": "ServiceAccount", "name": "go-reset"})
}

test_binding_ajeno_sa_control_plane_allowed if {
	count(deny) == 0 with input as rbg("default", {"kind": "ServiceAccount", "name": "go-reset", "namespace": "aqs-system"})
}

# U12: excepcion de warm-provisioner sobre secrets
test_warm_provisioner_secrets_create_allowed if {
	count(deny) == 0 with input as {
		"kind": "ClusterRole",
		"metadata": {"name": "warm-provisioner"},
		"rules": [{"apiGroups": [""], "resources": ["secrets"], "verbs": ["create"]}],
	}
}

test_warm_provisioner_secrets_get_denied if {
	count(deny) > 0 with input as {
		"kind": "ClusterRole",
		"metadata": {"name": "warm-provisioner"},
		"rules": [{"apiGroups": [""], "resources": ["secrets"], "verbs": ["get"]}],
	}
}

test_warm_provisioner_secrets_list_denied if {
	count(deny) > 0 with input as {
		"kind": "ClusterRole",
		"metadata": {"name": "warm-provisioner"},
		"rules": [{"apiGroups": [""], "resources": ["secrets"], "verbs": ["list"]}],
	}
}

test_warm_provisioner_secrets_create_and_get_denied if {
	count(deny) > 0 with input as {
		"kind": "ClusterRole",
		"metadata": {"name": "warm-provisioner"},
		"rules": [{"apiGroups": [""], "resources": ["secrets"], "verbs": ["create", "get"]}],
	}
}

test_other_role_secrets_create_denied if {
	count(deny) > 0 with input as {
		"kind": "Role",
		"metadata": {"name": "other-role", "namespace": "aqs-test"},
		"rules": [{"apiGroups": [""], "resources": ["secrets"], "verbs": ["create"]}],
	}
	count(deny) > 0 with input as {
		"kind": "Role",
		"metadata": {"name": "warm-provisioner", "namespace": "aqs-test"},
		"rules": [{"apiGroups": [""], "resources": ["secrets"], "verbs": ["create"]}],
	}
	count(deny) > 0 with input as {
		"kind": "ClusterRole",
		"metadata": {"name": "other-role"},
		"rules": [{"apiGroups": [""], "resources": ["secrets"], "verbs": ["create"]}],
	}
}

# U12: RoleBinding fuera de warm denegado para sujetos de aqs-w-*
test_binding_ajeno_sa_aqs_w_denied if {
	count(deny) > 0 with input as rbg("aqs-system", {"kind": "ServiceAccount", "name": "app", "namespace": "aqs-w-demo"})
}

test_binding_ajeno_user_sintetico_aqs_w_denied if {
	count(deny) > 0 with input as rbg("default", {"kind": "User", "name": "system:serviceaccount:aqs-w-demo:app"})
}

test_binding_ajeno_group_aqs_w_denied if {
	count(deny) > 0 with input as rbg("default", {"kind": "Group", "name": "system:serviceaccounts:aqs-w-demo"})
}
