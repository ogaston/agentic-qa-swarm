package provision

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/ogaston/agentic-qa-swarm/services/go-warm-manager/warmtarget"
)

// RandomHex genera una cadena hexadecimal criptográficamente segura de longitud n bytes (2n hex chars).
func RandomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "tok-" + hex.EncodeToString(b)
	}
	return hex.EncodeToString(b)
}

// WarmResources contiene todos los recursos de Kubernetes que componen un entorno warm.
type WarmResources struct {
	Namespace       *corev1.Namespace
	PolicyConfigMap *corev1.ConfigMap
	AppDeployment   *appsv1.Deployment
	AppService      *corev1.Service
	DBStatefulSet   *appsv1.StatefulSet
	DBService       *corev1.Service
	RedisDeployment *appsv1.Deployment
	RedisService    *corev1.Service
	ResourceQuota   *corev1.ResourceQuota
	LimitRange      *corev1.LimitRange
	NetworkPolicies []*networkingv1.NetworkPolicy
	DBCredentials   *corev1.Secret
	BaselineToken   *corev1.Secret
	StateConfigMap  *corev1.ConfigMap
}

// WorkloadTemplate genera la plantilla de recursos para un WarmTarget en su namespace aqs-w-<slug>.
func WorkloadTemplate(target warmtarget.WarmTarget) (*WarmResources, error) {
	ns := target.Namespace
	if ns == "" {
		return nil, fmt.Errorf("target sin namespace")
	}

	replicas1 := int32(1)
	fsGroup := int64(65532)
	defaultMode := int32(0440)
	udpProto := corev1.ProtocolUDP
	tcpProto := corev1.ProtocolTCP
	port53 := intstr.FromInt(53)

	res := &WarmResources{
		Namespace: &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: ns,
				Labels: map[string]string{
					"aqs.io/plane": "warm",
					"aqs.io/repo":  target.Repo,
				},
			},
		},
		PolicyConfigMap: &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "warm-policy",
				Namespace: ns,
			},
			Data: map[string]string{
				"idleScaleDownAfter": "30m",
				"minReplicasIdle":    "0",
			},
		},
		DBCredentials: &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "warm-db-credentials",
				Namespace: ns,
			},
			StringData: map[string]string{
				"password": RandomHex(16),
			},
		},
		BaselineToken: &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "target-app-baseline-token",
				Namespace: ns,
			},
			StringData: map[string]string{
				"token": RandomHex(16),
			},
		},
		AppDeployment: &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "warm-app",
				Namespace: ns,
				Labels: map[string]string{
					"app.kubernetes.io/name": "warm-app",
				},
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas1,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app.kubernetes.io/name": "warm-app",
					},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"app.kubernetes.io/name": "warm-app",
						},
					},
					Spec: corev1.PodSpec{
						SecurityContext: &corev1.PodSecurityContext{
							FSGroup: &fsGroup,
						},
						Containers: []corev1.Container{
							{
								Name:  "warm-app",
								Image: "nginx:1.27.2",
								Env: []corev1.EnvVar{
									{Name: "TZ", Value: "UTC"},
									{
										Name: "PGPASSWORD",
										ValueFrom: &corev1.EnvVarSource{
											SecretKeyRef: &corev1.SecretKeySelector{
												LocalObjectReference: corev1.LocalObjectReference{Name: "warm-db-credentials"},
												Key:                  "password",
											},
										},
									},
									{
										Name:  "DATABASE_URL",
										Value: fmt.Sprintf("postgres://postgres:$(PGPASSWORD)@warm-db.%s.svc:5432/postgres?sslmode=disable", ns),
									},
									{Name: "BASELINE_TOKEN_FILE", Value: "/etc/aqs/target-app-baseline/token"},
									{Name: "BASELINE_LISTEN_ADDR", Value: ":9090"},
								},
								Ports: []corev1.ContainerPort{
									{Name: "http", ContainerPort: 80},
									{Name: "baseline", ContainerPort: 9090},
								},
								VolumeMounts: []corev1.VolumeMount{
									{
										Name:      "target-app-baseline",
										MountPath: "/etc/aqs/target-app-baseline",
										ReadOnly:  true,
									},
								},
								ReadinessProbe: &corev1.Probe{
									ProbeHandler: corev1.ProbeHandler{
										HTTPGet: &corev1.HTTPGetAction{
											Path: "/",
											Port: intstr.FromString("http"),
										},
									},
								},
								LivenessProbe: &corev1.Probe{
									ProbeHandler: corev1.ProbeHandler{
										HTTPGet: &corev1.HTTPGetAction{
											Path: "/",
											Port: intstr.FromString("http"),
										},
									},
								},
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("50m"),
										corev1.ResourceMemory: resource.MustParse("64Mi"),
									},
									Limits: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("500m"),
										corev1.ResourceMemory: resource.MustParse("256Mi"),
									},
								},
							},
						},
						Volumes: []corev1.Volume{
							{
								Name: "target-app-baseline",
								VolumeSource: corev1.VolumeSource{
									Secret: &corev1.SecretVolumeSource{
										SecretName:  "target-app-baseline-token",
										DefaultMode: &defaultMode,
									},
								},
							},
						},
					},
				},
			},
		},
		AppService: &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "warm-app",
				Namespace: ns,
			},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{
					"app.kubernetes.io/name": "warm-app",
				},
				Ports: []corev1.ServicePort{
					{Name: "http", Port: 80, TargetPort: intstr.FromString("http")},
					{Name: "baseline", Port: 9090, TargetPort: intstr.FromString("baseline")},
				},
			},
		},
		DBStatefulSet: &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "warm-db",
				Namespace: ns,
				Labels: map[string]string{
					"app.kubernetes.io/name": "warm-db",
				},
			},
			Spec: appsv1.StatefulSetSpec{
				ServiceName: "warm-db",
				Replicas:    &replicas1,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app.kubernetes.io/name": "warm-db",
					},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"app.kubernetes.io/name": "warm-db",
						},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "postgres",
								Image: "postgres:16.4",
								Ports: []corev1.ContainerPort{
									{Name: "pg", ContainerPort: 5432},
								},
								Env: []corev1.EnvVar{
									{Name: "PGDATA", Value: "/var/lib/postgresql/data/pgdata"},
									{
										Name: "POSTGRES_PASSWORD",
										ValueFrom: &corev1.EnvVarSource{
											SecretKeyRef: &corev1.SecretKeySelector{
												LocalObjectReference: corev1.LocalObjectReference{Name: "warm-db-credentials"},
												Key:                  "password",
											},
										},
									},
								},
								ReadinessProbe: &corev1.Probe{
									ProbeHandler: corev1.ProbeHandler{
										Exec: &corev1.ExecAction{
											Command: []string{"pg_isready", "-U", "postgres"},
										},
									},
								},
								LivenessProbe: &corev1.Probe{
									InitialDelaySeconds: 30,
									ProbeHandler: corev1.ProbeHandler{
										Exec: &corev1.ExecAction{
											Command: []string{"pg_isready", "-U", "postgres"},
										},
									},
								},
								VolumeMounts: []corev1.VolumeMount{
									{Name: "data", MountPath: "/var/lib/postgresql/data"},
								},
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("100m"),
										corev1.ResourceMemory: resource.MustParse("128Mi"),
									},
									Limits: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("500m"),
										corev1.ResourceMemory: resource.MustParse("512Mi"),
									},
								},
							},
						},
					},
				},
				VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "data",
						},
						Spec: corev1.PersistentVolumeClaimSpec{
							AccessModes: []corev1.PersistentVolumeAccessMode{
								corev1.ReadWriteOnce,
							},
							Resources: corev1.VolumeResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: resource.MustParse("1Gi"),
								},
							},
						},
					},
				},
			},
		},
		DBService: &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "warm-db",
				Namespace: ns,
			},
			Spec: corev1.ServiceSpec{
				ClusterIP: corev1.ClusterIPNone,
				Selector: map[string]string{
					"app.kubernetes.io/name": "warm-db",
				},
				Ports: []corev1.ServicePort{
					{Name: "pg", Port: 5432, TargetPort: intstr.FromString("pg")},
				},
			},
		},
		RedisDeployment: &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "warm-redis",
				Namespace: ns,
				Labels: map[string]string{
					"app.kubernetes.io/name": "warm-redis",
				},
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas1,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app.kubernetes.io/name": "warm-redis",
					},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"app.kubernetes.io/name": "warm-redis",
						},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "redis",
								Image: "redis:7.4.1",
								Env: []corev1.EnvVar{
									{Name: "TZ", Value: "UTC"},
								},
								Ports: []corev1.ContainerPort{
									{Name: "redis", ContainerPort: 6379},
								},
								ReadinessProbe: &corev1.Probe{
									ProbeHandler: corev1.ProbeHandler{
										TCPSocket: &corev1.TCPSocketAction{
											Port: intstr.FromString("redis"),
										},
									},
								},
								LivenessProbe: &corev1.Probe{
									ProbeHandler: corev1.ProbeHandler{
										TCPSocket: &corev1.TCPSocketAction{
											Port: intstr.FromString("redis"),
										},
									},
								},
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("50m"),
										corev1.ResourceMemory: resource.MustParse("64Mi"),
									},
									Limits: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("250m"),
										corev1.ResourceMemory: resource.MustParse("128Mi"),
									},
								},
							},
						},
					},
				},
			},
		},
		RedisService: &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "warm-redis",
				Namespace: ns,
			},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{
					"app.kubernetes.io/name": "warm-redis",
				},
				Ports: []corev1.ServicePort{
					{Name: "redis", Port: 6379, TargetPort: intstr.FromString("redis")},
				},
			},
		},
		ResourceQuota: &corev1.ResourceQuota{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ns + "-quota",
				Namespace: ns,
			},
			Spec: corev1.ResourceQuotaSpec{
				Hard: corev1.ResourceList{
					corev1.ResourcePods:                     resource.MustParse("15"),
					corev1.ResourceName("count/jobs.batch"): resource.MustParse("100"),
					corev1.ResourceRequestsCPU:              resource.MustParse("2"),
					corev1.ResourceRequestsMemory:           resource.MustParse("3Gi"),
					corev1.ResourceLimitsCPU:                resource.MustParse("6"),
					corev1.ResourceLimitsMemory:             resource.MustParse("6Gi"),
				},
			},
		},
		LimitRange: &corev1.LimitRange{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ns + "-limits",
				Namespace: ns,
			},
			Spec: corev1.LimitRangeSpec{
				Limits: []corev1.LimitRangeItem{
					{
						Type: corev1.LimitTypeContainer,
						Default: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("250m"),
							corev1.ResourceMemory: resource.MustParse("256Mi"),
						},
						DefaultRequest: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("50m"),
							corev1.ResourceMemory: resource.MustParse("64Mi"),
						},
						Max: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1"),
							corev1.ResourceMemory: resource.MustParse("1Gi"),
						},
					},
				},
			},
		},
		NetworkPolicies: []*networkingv1.NetworkPolicy{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "default-deny",
					Namespace: ns,
				},
				Spec: networkingv1.NetworkPolicySpec{
					PodSelector: metav1.LabelSelector{},
					PolicyTypes: []networkingv1.PolicyType{
						networkingv1.PolicyTypeIngress,
						networkingv1.PolicyTypeEgress,
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "allow-same-namespace",
					Namespace: ns,
				},
				Spec: networkingv1.NetworkPolicySpec{
					PodSelector: metav1.LabelSelector{},
					PolicyTypes: []networkingv1.PolicyType{
						networkingv1.PolicyTypeIngress,
						networkingv1.PolicyTypeEgress,
					},
					Ingress: []networkingv1.NetworkPolicyIngressRule{
						{
							From: []networkingv1.NetworkPolicyPeer{
								{PodSelector: &metav1.LabelSelector{}},
							},
						},
					},
					Egress: []networkingv1.NetworkPolicyEgressRule{
						{
							To: []networkingv1.NetworkPolicyPeer{
								{PodSelector: &metav1.LabelSelector{}},
							},
						},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "allow-dns",
					Namespace: ns,
				},
				Spec: networkingv1.NetworkPolicySpec{
					PodSelector: metav1.LabelSelector{},
					PolicyTypes: []networkingv1.PolicyType{
						networkingv1.PolicyTypeEgress,
					},
					Egress: []networkingv1.NetworkPolicyEgressRule{
						{
							To: []networkingv1.NetworkPolicyPeer{
								{
									NamespaceSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"kubernetes.io/metadata.name": "kube-system",
										},
									},
								},
							},
							Ports: []networkingv1.NetworkPolicyPort{
								{Protocol: &udpProto, Port: &port53},
								{Protocol: &tcpProto, Port: &port53},
							},
						},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "allow-from-control-plane",
					Namespace: ns,
				},
				Spec: networkingv1.NetworkPolicySpec{
					PodSelector: metav1.LabelSelector{},
					PolicyTypes: []networkingv1.PolicyType{
						networkingv1.PolicyTypeIngress,
					},
					Ingress: []networkingv1.NetworkPolicyIngressRule{
						{
							From: []networkingv1.NetworkPolicyPeer{
								{
									NamespaceSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"kubernetes.io/metadata.name": "aqs-system",
										},
									},
								},
							},
						},
					},
				},
			},
		},
		StateConfigMap: &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "warm-state",
				Namespace: ns,
			},
			Data: map[string]string{
				"state": fmt.Sprintf(`{"warm_id":%q,"state":"ready","reset_verified":true,"baseline_version":"v1","namespace":%q,"repo":%q}`,
					target.WarmID, ns, target.Repo),
			},
		},
	}

	return res, nil
}
