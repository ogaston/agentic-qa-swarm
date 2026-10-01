package main

import rego.v1

sa(name, extra) := object.union({"apiVersion": "v1", "kind": "ServiceAccount", "metadata": {"name": name, "namespace": "aqs-test"}}, extra)

dep(name, tpl) := {"kind": "Deployment", "metadata": {"name": name, "namespace": "aqs-system"}, "spec": {"template": {"spec": tpl}}}

# ClusterRole / ClusterRoleBinding
test_clusterrolebinding_denied if {
	count(deny) > 0 with input as {"kind": "ClusterRoleBinding", "metadata": {"name": "x"}}
}

test_clusterrole_denied if {
	count(deny) > 0 with input as {"kind": "ClusterRole", "metadata": {"name": "x"}}
}

test_role_allowed if {
	count(deny) == 0 with input as {"kind": "Role", "metadata": {"name": "r", "namespace": "aqs-test"}}
}

# RoleBinding con aqs-runner
test_rolebinding_runner_denied if {
	count(deny) > 0 with input as {"kind": "RoleBinding", "metadata": {"name": "b", "namespace": "aqs-test"}, "subjects": [{"kind": "ServiceAccount", "name": "aqs-runner", "namespace": "aqs-test"}]}
}

test_rolebinding_reset_allowed if {
	count(deny) == 0 with input as {"kind": "RoleBinding", "metadata": {"name": "b", "namespace": "aqs-test"}, "subjects": [{"kind": "ServiceAccount", "name": "aqs-reset", "namespace": "aqs-test"}]}
}

# automount
test_runner_without_automount_denied if {
	count(deny) > 0 with input as sa("aqs-runner", {})
}

test_runner_automount_true_denied if {
	count(deny) > 0 with input as sa("aqs-runner", {"automountServiceAccountToken": true})
}

test_runner_automount_false_allowed if {
	count(deny) == 0 with input as sa("aqs-runner", {"automountServiceAccountToken": false})
}

# ipBlock
test_ipblock_denied if {
	count(deny) > 0 with input as {"kind": "NetworkPolicy", "metadata": {"name": "n", "namespace": "aqs-test"}, "spec": {"podSelector": {}, "egress": [{"to": [{"ipBlock": {"cidr": "0.0.0.0/0"}}]}]}}
}

test_netpol_without_ipblock_allowed if {
	count(deny) == 0 with input as {"kind": "NetworkPolicy", "metadata": {"name": "n", "namespace": "aqs-test"}, "spec": {"podSelector": {}, "egress": [{"to": [{"podSelector": {}}]}]}}
}

test_ipblock_outside_aqs_test_ignored if {
	count(deny) == 0 with input as {"kind": "NetworkPolicy", "metadata": {"name": "n", "namespace": "otro"}, "spec": {"podSelector": {}, "egress": [{"to": [{"ipBlock": {"cidr": "0.0.0.0/0"}}]}]}}
}

# serviceAccountName
test_cronjob_without_sa_denied if {
	count(deny) > 0 with input as {"kind": "CronJob", "metadata": {"name": "c", "namespace": "aqs-test"}, "spec": {"jobTemplate": {"spec": {"template": {"spec": {}}}}}}
}

test_cronjob_with_sa_allowed if {
	count(deny) == 0 with input as {"kind": "CronJob", "metadata": {"name": "c", "namespace": "aqs-test"}, "spec": {"jobTemplate": {"spec": {"template": {"spec": {"serviceAccountName": "aqs-reset"}}}}}}
}

test_job_without_sa_denied if {
	count(deny) > 0 with input as {"kind": "Job", "metadata": {"name": "j", "namespace": "aqs-test"}, "spec": {"template": {"spec": {}}}}
}

test_cp_deployment_without_sa_denied if {
	count(deny) > 0 with input as dep("go-reset", {})
}

test_cp_deployment_with_sa_allowed if {
	count(deny) == 0 with input as dep("go-reset", {"serviceAccountName": "go-reset"})
}

test_other_deployment_without_sa_allowed if {
	count(deny) == 0 with input as dep("ui-api", {})
}

# default-deny (conjunto, --combine)
dd := {"kind": "NetworkPolicy", "metadata": {"name": "default-deny", "namespace": "aqs-test"}, "spec": {"podSelector": {}, "policyTypes": ["Ingress", "Egress"]}}

test_missing_default_deny_denied if {
	count(deny) > 0 with input as [{"path": "x", "contents": {"kind": "ConfigMap", "metadata": {"name": "c"}}}]
}

test_default_deny_only_egress_denied if {
	count(deny) > 0 with input as [{"path": "x", "contents": object.union(dd, {"spec": {"policyTypes": ["Egress"]}})}]
}

test_default_deny_present_allowed if {
	count(deny) == 0 with input as [{"path": "x", "contents": dd}]
}
