package main

import rego.v1

cp_dep(name, spec) := {"kind": "Deployment", "metadata": {"name": name, "namespace": "aqs-system"}, "spec": {"template": {"spec": spec}}}

wl_pod(kind, spec) := {"kind": kind, "metadata": {"name": "w", "namespace": "aqs-test"}, "spec": {"template": {"spec": spec}}} if kind != "CronJob"

wl_pod("CronJob", spec) := {"kind": "CronJob", "metadata": {"name": "w", "namespace": "aqs-test"}, "spec": {"jobTemplate": {"spec": {"template": {"spec": spec}}}}}

test_cp_sa_vacio_denied if {
	count(deny) > 0 with input as cp_dep("ui-api", {"serviceAccountName": "", "automountServiceAccountToken": false})
}

test_cp_sa_null_denied if {
	count(deny) > 0 with input as cp_dep("go-reset", {"serviceAccountName": null})
}

test_cp_sa_numero_denied if {
	count(deny) > 0 with input as cp_dep("go-governance", {"serviceAccountName": 5, "automountServiceAccountToken": false})
}

test_cp_sa_ausente_denied_en_los_siete if {
	every n in {"ui-api", "go-intake", "go-governance", "go-identity", "go-run-controller", "go-warm-manager", "go-reset"} {
		count(deny) > 0 with input as cp_dep(n, {"automountServiceAccountToken": false})
	}
}

test_cp_sa_valido_allowed if {
	count(deny) == 0 with input as cp_dep("go-identity", {"serviceAccountName": "go-identity", "automountServiceAccountToken": false})
	count(deny) == 0 with input as cp_dep("go-reset", {"serviceAccountName": "go-reset"})
}

test_cp_nuevos_sin_automount_denied if {
	count(deny) > 0 with input as cp_dep("go-intake", {"serviceAccountName": "go-intake"})
	count(deny) > 0 with input as cp_dep("go-intake", {"serviceAccountName": "go-intake", "automountServiceAccountToken": true})
}

test_automount_true_en_aqs_test_denied if {
	every k in {"Deployment", "StatefulSet", "Job", "CronJob"} {
		count(deny) > 0 with input as wl_pod(k, {"serviceAccountName": "aqs-runner", "automountServiceAccountToken": true})
	}
}

test_automount_false_en_aqs_test_allowed if {
	every k in {"Deployment", "StatefulSet", "Job", "CronJob"} {
		count(deny) == 0 with input as wl_pod(k, {"serviceAccountName": "aqs-runner", "automountServiceAccountToken": false})
	}
}

test_host_namespaces_en_aqs_test_denied if {
	every c in {"hostNetwork", "hostPID", "hostIPC"} {
		count(deny) > 0 with input as wl_pod("Deployment", {"serviceAccountName": "aqs-runner", c: true})
	}
}

test_hostnetwork_string_denied if {
	count(deny) > 0 with input as wl_pod("Job", {"serviceAccountName": "aqs-runner", "hostNetwork": "true"})
}

test_hostnetwork_false_allowed if {
	count(deny) == 0 with input as wl_pod("Deployment", {"serviceAccountName": "aqs-runner", "hostNetwork": false})
}
