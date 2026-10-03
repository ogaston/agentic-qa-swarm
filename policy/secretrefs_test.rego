package main

import rego.v1

sr_pod(env) := {"containers": [{"name": "c", "image": "x:1", "env": env}]}

sr_dep(ns, env) := {"kind": "Deployment", "metadata": {"name": "x", "namespace": ns}, "spec": {"template": {"spec": sr_pod(env)}}}

sr_wl(kind, env) := {"kind": kind, "metadata": {"name": "x", "namespace": "aqs-system"}, "spec": {"template": {"spec": sr_pod(env)}}} if kind != "CronJob"

sr_wl("CronJob", env) := {"kind": "CronJob", "metadata": {"name": "x", "namespace": "aqs-system"}, "spec": {"jobTemplate": {"spec": {"template": {"spec": sr_pod(env)}}}}}

test_sr_literal_token_denied if {
	count(deny) > 0 with input as sr_dep("aqs-system", [{"name": "FOO_TOKEN", "value": "abc"}])
}

test_sr_literal_secret_denied if {
	count(deny) > 0 with input as sr_dep("aqs-system", [{"name": "FOO_SECRET", "value": "abc"}])
}

test_sr_literal_password_denied if {
	count(deny) > 0 with input as sr_dep("aqs-system", [{"name": "DB_PASSWORD", "value": "abc"}])
}

test_sr_literal_passwd_denied if {
	count(deny) > 0 with input as sr_dep("aqs-system", [{"name": "DB_PASSWD", "value": "abc"}])
}

test_sr_literal_secret_key_denied if {
	count(deny) > 0 with input as sr_dep("aqs-system", [{"name": "KMS_SECRET_KEY", "value": "abc"}])
}

test_sr_vacio_denied if {
	count(deny) > 0 with input as sr_dep("aqs-system", [{"name": "FOO_TOKEN", "value": ""}])
}

test_sr_token_valuefrom_allowed if {
	count(deny) == 0 with input as sr_dep("aqs-system", [{"name": "FOO_TOKEN", "valueFrom": {"secretKeyRef": {"name": "s", "key": "token"}}}])
}

test_sr_cada_sufijo_valuefrom_allowed if {
	every n in {"A_TOKEN", "A_SECRET", "A_PASSWORD", "A_PASSWD", "A_SECRET_KEY"} {
		count(deny) == 0 with input as sr_dep("aqs-system", [{"name": n, "valueFrom": {"secretKeyRef": {"name": "s", "key": "k"}}}])
	}
}

test_sr_nombre_parecido_allowed if {
	count(deny) == 0 with input as sr_dep("aqs-system", [{"name": "TOKEN_TTL", "value": "60"}, {"name": "SECRETARY", "value": "x"}, {"name": "PASSWORD_MIN_LEN", "value": "8"}])
}

test_sr_varias_variables_una_violacion_denied if {
	count(deny) == 1 with input as sr_dep("aqs-system", [{"name": "TZ", "value": "UTC"}, {"name": "A_TOKEN", "value": "abc"}, {"name": "B_TOKEN", "valueFrom": {"secretKeyRef": {"name": "s", "key": "k"}}}])
}

test_sr_statefulset_denied if {
	count(deny) > 0 with input as sr_wl("StatefulSet", [{"name": "A_SECRET", "value": "abc"}])
}

test_sr_job_denied if {
	count(deny) > 0 with input as sr_wl("Job", [{"name": "A_PASSWORD", "value": "abc"}])
}

test_sr_cronjob_denied if {
	count(deny) > 0 with input as sr_wl("CronJob", [{"name": "A_TOKEN", "value": "abc"}])
}

test_sr_fuera_de_aqs_system_no_evalua if {
	count(deny) == 0 with input as sr_dep("aqs-test", [{"name": "FOO_TOKEN", "value": "abc"}])
}

test_sr_initcontainer_deployment_denied if {
	count(deny) > 0 with input as {"kind": "Deployment", "metadata": {"name": "x", "namespace": "aqs-system"}, "spec": {"template": {"spec": {"containers": [{"name": "c", "image": "x:1"}], "initContainers": [{"name": "i", "image": "x:1", "env": [{"name": "A_TOKEN", "value": "abc"}]}]}}}}
}

test_sr_initcontainer_valuefrom_allowed if {
	count(deny) == 0 with input as {"kind": "Deployment", "metadata": {"name": "x", "namespace": "aqs-system"}, "spec": {"template": {"spec": {"containers": [{"name": "c", "image": "x:1"}], "initContainers": [{"name": "i", "image": "x:1", "env": [{"name": "A_TOKEN", "valueFrom": {"secretKeyRef": {"name": "s", "key": "k"}}}]}]}}}}
}

test_sr_initcontainer_cronjob_denied if {
	count(deny) > 0 with input as {"kind": "CronJob", "metadata": {"name": "x", "namespace": "aqs-system"}, "spec": {"jobTemplate": {"spec": {"template": {"spec": {"containers": [{"name": "c", "image": "x:1"}], "initContainers": [{"name": "i", "image": "x:1", "env": [{"name": "A_SECRET", "value": "abc"}]}]}}}}}}
}

test_sr_initcontainer_cronjob_valuefrom_allowed if {
	count(deny) == 0 with input as {"kind": "CronJob", "metadata": {"name": "x", "namespace": "aqs-system"}, "spec": {"jobTemplate": {"spec": {"template": {"spec": {"containers": [{"name": "c", "image": "x:1"}], "initContainers": [{"name": "i", "image": "x:1", "env": [{"name": "A_SECRET", "valueFrom": {"secretKeyRef": {"name": "s", "key": "k"}}}]}]}}}}}}
}

test_sr_ephemeral_denied if {
	count(deny) > 0 with input as {"kind": "Deployment", "metadata": {"name": "x", "namespace": "aqs-system"}, "spec": {"template": {"spec": {"containers": [{"name": "c", "image": "x:1"}], "ephemeralContainers": [{"name": "e", "image": "x:1", "env": [{"name": "A_PASSWD", "value": "abc"}]}]}}}}
}

test_sr_ephemeral_valuefrom_allowed if {
	count(deny) == 0 with input as {"kind": "Deployment", "metadata": {"name": "x", "namespace": "aqs-system"}, "spec": {"template": {"spec": {"containers": [{"name": "c", "image": "x:1"}], "ephemeralContainers": [{"name": "e", "image": "x:1", "env": [{"name": "A_PASSWD", "valueFrom": {"secretKeyRef": {"name": "s", "key": "k"}}}]}]}}}}
}

test_sr_initcontainer_fuera_de_aqs_system_no_evalua if {
	count(deny) == 0 with input as {"kind": "Deployment", "metadata": {"name": "x", "namespace": "aqs-test"}, "spec": {"template": {"spec": {"containers": [{"name": "c", "image": "x:1"}], "initContainers": [{"name": "i", "image": "x:1", "env": [{"name": "A_TOKEN", "value": "abc"}]}]}}}}
}
