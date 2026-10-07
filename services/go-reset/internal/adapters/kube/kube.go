// Package kube es el adaptador client-go de KubeAPI y StateStore. Solo opera en aqs-test.
package kube

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/ogaston/agentic-qa-swarm/services/go-reset/internal/core"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

// Namespace es el único namespace permitido.
const Namespace = "aqs-test"

const (
	appName       = "warm-app"
	policyName    = "warm-policy"
	stateName     = "warm-state"
	stateKey      = "state"
	updatedAtKey  = "updated_at"
	appLabelValue = "app.kubernetes.io/name=warm-app"
)

// Client implementa core.KubeAPI y core.StateStore.
type Client struct {
	cs  kubernetes.Interface
	ns  string
	now func() time.Time
}

// New rechaza cualquier namespace distinto de aqs-test.
func New(cs kubernetes.Interface, ns string, now func() time.Time) (*Client, error) {
	if ns != Namespace {
		return nil, fmt.Errorf("namespace %q no permitido: solo %s", ns, Namespace)
	}
	if now == nil {
		now = time.Now
	}
	return &Client{cs: cs, ns: ns, now: now}, nil
}

func (c *Client) Replicas(ctx context.Context) (int32, error) {
	d, err := c.cs.AppsV1().Deployments(c.ns).Get(ctx, appName, metav1.GetOptions{})
	if err != nil {
		return 0, err
	}
	if d.Spec.Replicas == nil {
		return 1, nil
	}
	return *d.Spec.Replicas, nil
}

func (c *Client) RestartApp(ctx context.Context) error {
	patch := fmt.Sprintf(`{"spec":{"template":{"metadata":{"annotations":{"aqs/restarted-at":%q}}}}}`,
		c.now().UTC().Format(time.RFC3339Nano))
	_, err := c.cs.AppsV1().Deployments(c.ns).Patch(ctx, appName, types.StrategicMergePatchType, []byte(patch), metav1.PatchOptions{})
	return err
}

func (c *Client) ScaleApp(ctx context.Context, n int32) error {
	patch := `{"spec":{"replicas":` + strconv.Itoa(int(n)) + `}}`
	_, err := c.cs.AppsV1().Deployments(c.ns).Patch(ctx, appName, types.StrategicMergePatchType, []byte(patch), metav1.PatchOptions{})
	return err
}

func (c *Client) deletePods(ctx context.Context) error {
	pl, err := c.cs.CoreV1().Pods(c.ns).List(ctx, metav1.ListOptions{LabelSelector: appLabelValue})
	if err != nil {
		return err
	}
	for _, p := range pl.Items {
		if err := c.cs.CoreV1().Pods(c.ns).Delete(ctx, p.Name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

// Rebuild reinicia y borra los pods para que vuelvan desde la imagen base.
func (c *Client) Rebuild(ctx context.Context) error {
	if err := c.RestartApp(ctx); err != nil {
		return err
	}
	return c.deletePods(ctx)
}

// Teardown destruye (escala a 0 y borra pods) y reprovisiona (escala a 1).
func (c *Client) Teardown(ctx context.Context) error {
	if err := c.ScaleApp(ctx, 0); err != nil {
		return err
	}
	if err := c.deletePods(ctx); err != nil {
		return err
	}
	return c.ScaleApp(ctx, 1)
}

// AppReady lee los pods reales: al menos uno, todos Running y Ready, ninguno terminando.
func (c *Client) AppReady(ctx context.Context) (bool, error) {
	pl, err := c.cs.CoreV1().Pods(c.ns).List(ctx, metav1.ListOptions{LabelSelector: appLabelValue})
	if err != nil {
		return false, err
	}
	if len(pl.Items) == 0 {
		return false, nil
	}
	for _, p := range pl.Items {
		if p.DeletionTimestamp != nil || p.Status.Phase != corev1.PodRunning || !podReady(p) {
			return false, nil
		}
	}
	return true, nil
}

func podReady(p corev1.Pod) bool {
	for _, cd := range p.Status.Conditions {
		if cd.Type == corev1.PodReady {
			return cd.Status == corev1.ConditionTrue
		}
	}
	return false
}

func (c *Client) WarmPolicy(ctx context.Context) (core.Policy, error) {
	cm, err := c.cs.CoreV1().ConfigMaps(c.ns).Get(ctx, policyName, metav1.GetOptions{})
	if err != nil {
		return core.Policy{}, err
	}
	d, err := time.ParseDuration(cm.Data["idleScaleDownAfter"])
	if err != nil {
		return core.Policy{}, fmt.Errorf("idleScaleDownAfter: %w", err)
	}
	n, err := strconv.Atoi(cm.Data["minReplicasIdle"])
	if err != nil || n < 0 {
		return core.Policy{}, fmt.Errorf("minReplicasIdle inválido")
	}
	return core.Policy{IdleScaleDownAfter: d, MinReplicasIdle: int32(n)}, nil
}

// Get lee el ConfigMap warm-state (clave "state" = WarmState JSON; "updated_at" = RFC3339).
func (c *Client) Get(ctx context.Context) (core.Snapshot, bool, error) {
	cm, err := c.cs.CoreV1().ConfigMaps(c.ns).Get(ctx, stateName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return core.Snapshot{}, false, nil
	}
	if err != nil {
		return core.Snapshot{}, false, err
	}
	var w core.WarmState
	if err := json.Unmarshal([]byte(cm.Data[stateKey]), &w); err != nil {
		return core.Snapshot{}, false, fmt.Errorf("warm-state ilegible: %w", err)
	}
	at, _ := time.Parse(time.RFC3339Nano, cm.Data[updatedAtKey])
	return core.Snapshot{WarmState: w, UpdatedAt: at}, true, nil
}

// Put escribe (crea o actualiza) el ConfigMap warm-state.
func (c *Client) Put(ctx context.Context, w core.WarmState, at time.Time) error {
	b, _ := json.Marshal(w)
	data := map[string]string{stateKey: string(b), updatedAtKey: at.UTC().Format(time.RFC3339Nano)}
	cms := c.cs.CoreV1().ConfigMaps(c.ns)
	cm, err := cms.Get(ctx, stateName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = cms.Create(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: stateName, Namespace: c.ns}, Data: data}, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	cm.Data = data
	_, err = cms.Update(ctx, cm, metav1.UpdateOptions{})
	return err
}
