package provision

import (
	"context"
	"fmt"
	"log/slog"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/ogaston/agentic-qa-swarm/services/go-warm-manager/warmtarget"
)

// Reconciler reconcilia de forma idempotente los entornos warm deseados sobre el clúster.
type Reconciler struct {
	Kube kubernetes.Interface
	Log  *slog.Logger
}

// ReconcileResult resume las acciones tomadas en la corrida de reconciliación.
type ReconcileResult struct {
	CreatedNamespaces []string
	DeletedNamespaces []string
	EnsuredTargets    []string
}

// Reconcile reconcilia de forma idempotente una lista de WarmTargets deseados:
// 1. Crea namespaces y recursos faltantes para cada target.
// 2. Elimina namespaces de repos ya no deseados (conservando siempre el legacy aqs-test).
func (r *Reconciler) Reconcile(ctx context.Context, targets []warmtarget.WarmTarget) (*ReconcileResult, error) {
	res := &ReconcileResult{
		CreatedNamespaces: []string{},
		DeletedNamespaces: []string{},
		EnsuredTargets:    []string{},
	}

	desiredNS := make(map[string]warmtarget.WarmTarget)
	for _, t := range targets {
		if t.Namespace != "" {
			desiredNS[t.Namespace] = t
		}
	}

	// 1. Descubrir namespaces existentes con plane=warm
	nss, err := r.Kube.CoreV1().Namespaces().List(ctx, metav1.ListOptions{LabelSelector: "aqs.io/plane=warm"})
	if err != nil {
		return nil, fmt.Errorf("listando namespaces warm: %w", err)
	}

	// 2. Eliminar namespaces de repos removidos (NUNCA eliminar aqs-test)
	for _, ns := range nss.Items {
		if ns.Name == warmtarget.LegacyNamespace {
			continue
		}
		if _, ok := desiredNS[ns.Name]; !ok {
			if err := r.Kube.CoreV1().Namespaces().Delete(ctx, ns.Name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
				return nil, fmt.Errorf("eliminando namespace %s: %w", ns.Name, err)
			}
			res.DeletedNamespaces = append(res.DeletedNamespaces, ns.Name)
		}
	}

	// 3. Crear / asegurar recursos para cada target deseado
	for _, t := range targets {
		if t.Namespace == "" || t.Namespace == warmtarget.LegacyNamespace {
			continue
		}
		resources, err := WorkloadTemplate(t)
		if err != nil {
			return nil, fmt.Errorf("generando plantilla para %s: %w", t.Repo, err)
		}

		created, err := r.ensureNamespace(ctx, resources.Namespace)
		if err != nil {
			return nil, err
		}
		if created {
			res.CreatedNamespaces = append(res.CreatedNamespaces, t.Namespace)
		}

		if err := r.ensureConfigMap(ctx, resources.PolicyConfigMap); err != nil {
			return nil, err
		}
		if err := r.ensureConfigMap(ctx, resources.StateConfigMap); err != nil {
			return nil, err
		}

		if err := r.ensureSecret(ctx, resources.DBCredentials); err != nil {
			return nil, err
		}
		if err := r.ensureSecret(ctx, resources.BaselineToken); err != nil {
			return nil, err
		}

		if err := r.ensureDeployment(ctx, resources.AppDeployment); err != nil {
			return nil, err
		}
		if err := r.ensureDeployment(ctx, resources.RedisDeployment); err != nil {
			return nil, err
		}

		if err := r.ensureStatefulSet(ctx, resources.DBStatefulSet); err != nil {
			return nil, err
		}

		if err := r.ensureService(ctx, resources.AppService); err != nil {
			return nil, err
		}
		if err := r.ensureService(ctx, resources.DBService); err != nil {
			return nil, err
		}
		if err := r.ensureService(ctx, resources.RedisService); err != nil {
			return nil, err
		}

		if err := r.ensureResourceQuota(ctx, resources.ResourceQuota); err != nil {
			return nil, err
		}
		if err := r.ensureLimitRange(ctx, resources.LimitRange); err != nil {
			return nil, err
		}

		for _, np := range resources.NetworkPolicies {
			if err := r.ensureNetworkPolicy(ctx, np); err != nil {
				return nil, err
			}
		}

		res.EnsuredTargets = append(res.EnsuredTargets, t.Repo)
	}

	return res, nil
}

func (r *Reconciler) ensureNamespace(ctx context.Context, ns *corev1.Namespace) (bool, error) {
	_, err := r.Kube.CoreV1().Namespaces().Get(ctx, ns.Name, metav1.GetOptions{})
	if err == nil {
		return false, nil
	}
	if !apierrors.IsNotFound(err) {
		return false, fmt.Errorf("obteniendo namespace %s: %w", ns.Name, err)
	}
	_, err = r.Kube.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return false, fmt.Errorf("creando namespace %s: %w", ns.Name, err)
	}
	return true, nil
}

func (r *Reconciler) ensureConfigMap(ctx context.Context, cm *corev1.ConfigMap) error {
	_, err := r.Kube.CoreV1().ConfigMaps(cm.Namespace).Get(ctx, cm.Name, metav1.GetOptions{})
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("obteniendo ConfigMap %s/%s: %w", cm.Namespace, cm.Name, err)
	}
	_, err = r.Kube.CoreV1().ConfigMaps(cm.Namespace).Create(ctx, cm, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("creando ConfigMap %s/%s: %w", cm.Namespace, cm.Name, err)
	}
	return nil
}

func (r *Reconciler) ensureSecret(ctx context.Context, sec *corev1.Secret) error {
	// go-warm-manager solo tiene permiso CREATE en secrets (no get/list por principio de mínimo privilegio).
	// La idempotencia se garantiza intentando directamente Create e ignorando AlreadyExists.
	_, err := r.Kube.CoreV1().Secrets(sec.Namespace).Create(ctx, sec, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("creando Secret %s/%s: %w", sec.Namespace, sec.Name, err)
	}
	return nil
}

func (r *Reconciler) ensureDeployment(ctx context.Context, dep *appsv1.Deployment) error {
	_, err := r.Kube.AppsV1().Deployments(dep.Namespace).Get(ctx, dep.Name, metav1.GetOptions{})
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("obteniendo Deployment %s/%s: %w", dep.Namespace, dep.Name, err)
	}
	_, err = r.Kube.AppsV1().Deployments(dep.Namespace).Create(ctx, dep, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("creando Deployment %s/%s: %w", dep.Namespace, dep.Name, err)
	}
	return nil
}

func (r *Reconciler) ensureStatefulSet(ctx context.Context, sts *appsv1.StatefulSet) error {
	_, err := r.Kube.AppsV1().StatefulSets(sts.Namespace).Get(ctx, sts.Name, metav1.GetOptions{})
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("obteniendo StatefulSet %s/%s: %w", sts.Namespace, sts.Name, err)
	}
	_, err = r.Kube.AppsV1().StatefulSets(sts.Namespace).Create(ctx, sts, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("creando StatefulSet %s/%s: %w", sts.Namespace, sts.Name, err)
	}
	return nil
}

func (r *Reconciler) ensureService(ctx context.Context, svc *corev1.Service) error {
	_, err := r.Kube.CoreV1().Services(svc.Namespace).Get(ctx, svc.Name, metav1.GetOptions{})
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("obteniendo Service %s/%s: %w", svc.Namespace, svc.Name, err)
	}
	_, err = r.Kube.CoreV1().Services(svc.Namespace).Create(ctx, svc, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("creando Service %s/%s: %w", svc.Namespace, svc.Name, err)
	}
	return nil
}

func (r *Reconciler) ensureResourceQuota(ctx context.Context, rq *corev1.ResourceQuota) error {
	_, err := r.Kube.CoreV1().ResourceQuotas(rq.Namespace).Get(ctx, rq.Name, metav1.GetOptions{})
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("obteniendo ResourceQuota %s/%s: %w", rq.Namespace, rq.Name, err)
	}
	_, err = r.Kube.CoreV1().ResourceQuotas(rq.Namespace).Create(ctx, rq, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("creando ResourceQuota %s/%s: %w", rq.Namespace, rq.Name, err)
	}
	return nil
}

func (r *Reconciler) ensureLimitRange(ctx context.Context, lr *corev1.LimitRange) error {
	_, err := r.Kube.CoreV1().LimitRanges(lr.Namespace).Get(ctx, lr.Name, metav1.GetOptions{})
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("obteniendo LimitRange %s/%s: %w", lr.Namespace, lr.Name, err)
	}
	_, err = r.Kube.CoreV1().LimitRanges(lr.Namespace).Create(ctx, lr, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("creando LimitRange %s/%s: %w", lr.Namespace, lr.Name, err)
	}
	return nil
}

func (r *Reconciler) ensureNetworkPolicy(ctx context.Context, np *networkingv1.NetworkPolicy) error {
	_, err := r.Kube.NetworkingV1().NetworkPolicies(np.Namespace).Get(ctx, np.Name, metav1.GetOptions{})
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("obteniendo NetworkPolicy %s/%s: %w", np.Namespace, np.Name, err)
	}
	_, err = r.Kube.NetworkingV1().NetworkPolicies(np.Namespace).Create(ctx, np, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("creando NetworkPolicy %s/%s: %w", np.Namespace, np.Name, err)
	}
	return nil
}
