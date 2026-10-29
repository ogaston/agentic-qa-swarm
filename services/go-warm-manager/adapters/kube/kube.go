// Package kube contiene los adaptadores de go-warm-manager sobre client-go.
package kube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"

	wm "github.com/ogaston/agentic-qa-swarm/services/go-warm-manager"
	"github.com/ogaston/agentic-qa-swarm/services/go-warm-manager/warmtarget"
)

// StateConfigMap es el ConfigMap que persiste el WarmState.
const StateConfigMap = "warm-state"

// StateStore guarda el WarmState en el ConfigMap warm-state del namespace de prueba.
type StateStore struct {
	C         kubernetes.Interface
	Seed      wm.WarmState // se usa si el ConfigMap no existe
	Namespace string       // namespace por defecto (wm.LegacyNamespace si está vacío)
}

func (s *StateStore) ns(override ...string) string {
	if len(override) > 0 && override[0] != "" {
		return override[0]
	}
	if s.Namespace != "" {
		return s.Namespace
	}
	return wm.LegacyNamespace
}

// Get lee el ConfigMap (o devuelve la semilla si no existe) en el namespace por defecto.
func (s *StateStore) Get(ctx context.Context) (wm.WarmState, error) {
	return s.getInNamespace(ctx, s.ns(), "", "")
}

// GetForTarget lee el WarmState correspondiente a un target.
func (s *StateStore) GetForTarget(ctx context.Context, target warmtarget.WarmTarget) (wm.WarmState, error) {
	return s.getInNamespace(ctx, target.Namespace, target.WarmID, target.Repo)
}

func (s *StateStore) getInNamespace(ctx context.Context, ns, warmID, repo string) (wm.WarmState, error) {
	if ns == "" {
		ns = s.ns()
	}
	cm, err := s.C.CoreV1().ConfigMaps(ns).Get(ctx, StateConfigMap, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		seed := s.Seed
		if warmID != "" {
			seed.WarmID = warmID
		}
		if repo != "" {
			seed.Repo = repo
		}
		if seed.Namespace == "" && ns != wm.LegacyNamespace && ns != "" {
			seed.Namespace = ns
		}
		return seed, nil
	}
	if err != nil {
		return wm.WarmState{}, err
	}
	var w wm.WarmState
	dec := json.NewDecoder(bytesReader(cm.Data["state"]))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&w); err != nil || !w.Valid() {
		return wm.WarmState{}, fmt.Errorf("warm-state corrupto: %v", err)
	}
	return w, nil
}

// CompareAndSwap escribe next solo si el estado persistido es expect.
func (s *StateStore) CompareAndSwap(ctx context.Context, expect, next wm.WarmState) error {
	ns := next.Namespace
	if ns == "" {
		ns = s.ns()
	}
	return s.casInNamespace(ctx, ns, expect, next)
}

// CompareAndSwapForTarget escribe next condicionalmente en el namespace del target.
func (s *StateStore) CompareAndSwapForTarget(ctx context.Context, target warmtarget.WarmTarget, expect, next wm.WarmState) error {
	ns := target.Namespace
	if ns == "" {
		ns = s.ns()
	}
	return s.casInNamespace(ctx, ns, expect, next)
}

func (s *StateStore) casInNamespace(ctx context.Context, ns string, expect, next wm.WarmState) error {
	if !next.Valid() {
		return errors.New("WarmState invalido")
	}
	raw, _ := json.Marshal(next)
	api := s.C.CoreV1().ConfigMaps(ns)
	cm, err := api.Get(ctx, StateConfigMap, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		if s.Seed != expect {
			return wm.ErrStateConflict
		}
		_, err = api.Create(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: StateConfigMap, Namespace: ns},
			Data: map[string]string{"state": string(raw)}}, metav1.CreateOptions{})
		if apierrors.IsAlreadyExists(err) {
			return wm.ErrStateConflict
		}
		return err
	}
	if err != nil {
		return err
	}
	var cur wm.WarmState
	if err := json.Unmarshal([]byte(cm.Data["state"]), &cur); err != nil || cur != expect {
		return wm.ErrStateConflict
	}
	cm.Data["state"] = string(raw)
	if _, err := api.Update(ctx, cm, metav1.UpdateOptions{}); err != nil {
		if apierrors.IsConflict(err) {
			return wm.ErrStateConflict
		}
		return err
	}
	return nil
}

// Put escribe (crea o actualiza) el ConfigMap.
func (s *StateStore) Put(ctx context.Context, w wm.WarmState) error {
	ns := w.Namespace
	if ns == "" {
		ns = s.ns()
	}
	return s.putInNamespace(ctx, ns, w)
}

// PutForTarget escribe el WarmState en el namespace del target.
func (s *StateStore) PutForTarget(ctx context.Context, target warmtarget.WarmTarget, w wm.WarmState) error {
	ns := target.Namespace
	if ns == "" {
		ns = s.ns()
	}
	return s.putInNamespace(ctx, ns, w)
}

func (s *StateStore) putInNamespace(ctx context.Context, ns string, w wm.WarmState) error {
	if !w.Valid() {
		return errors.New("WarmState invalido")
	}
	raw, _ := json.Marshal(w)
	api := s.C.CoreV1().ConfigMaps(ns)
	cm, err := api.Get(ctx, StateConfigMap, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = api.Create(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: StateConfigMap, Namespace: ns},
			Data: map[string]string{"state": string(raw)}}, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	if cm.Data == nil {
		cm.Data = map[string]string{}
	}
	cm.Data["state"] = string(raw)
	_, err = api.Update(ctx, cm, metav1.UpdateOptions{})
	return err
}

// LookupState busca el WarmState por warm_id o namespace. Con fallback legacy a LegacyNamespace.
func (s *StateStore) LookupState(ctx context.Context, idOrNS string) (wm.WarmState, error) {
	if idOrNS == "" || idOrNS == wm.LegacyNamespace || idOrNS == warmtarget.LegacyWarmID {
		return s.Get(ctx)
	}
	if warmtarget.ValidWarmNamespace(idOrNS) {
		return s.getInNamespace(ctx, idOrNS, "", "")
	}
	if strings.HasPrefix(idOrNS, "w-") {
		slug := strings.TrimPrefix(idOrNS, "w-")
		ns := "aqs-w-" + slug
		return s.getInNamespace(ctx, ns, idOrNS, "")
	}
	nss, err := s.C.CoreV1().Namespaces().List(ctx, metav1.ListOptions{LabelSelector: "aqs.io/plane=warm"})
	if err == nil {
		for _, item := range nss.Items {
			if item.Labels["aqs.io/repo"] == idOrNS {
				return s.getInNamespace(ctx, item.Name, "", item.Labels["aqs.io/repo"])
			}
			st, err := s.getInNamespace(ctx, item.Name, "", "")
			if err == nil && (st.WarmID == idOrNS || st.Namespace == idOrNS || st.Repo == idOrNS) {
				return st, nil
			}
		}
	}
	return s.Get(ctx)
}

// Health considera sano el warm si los pods de app, DB y Redis están Ready.
type Health struct {
	C         kubernetes.Interface
	Namespace string
}

func (h *Health) ns(override ...string) string {
	if len(override) > 0 && override[0] != "" {
		return override[0]
	}
	if h.Namespace != "" {
		return h.Namespace
	}
	return wm.LegacyNamespace
}

var warmComponents = []string{"warm-app", "warm-db", "warm-redis"}

// Check falla si falta algún pod Ready de los tres componentes.
func (h *Health) Check(ctx context.Context) error {
	return h.CheckInNamespace(ctx, h.ns())
}

// CheckInNamespace comprueba los componentes en un namespace dado.
func (h *Health) CheckInNamespace(ctx context.Context, ns string) error {
	if ns == "" {
		ns = h.ns()
	}
	for _, comp := range warmComponents {
		pods, err := h.C.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=" + comp})
		if err != nil {
			return err
		}
		ready := false
		for _, p := range pods.Items {
			for _, c := range p.Status.Conditions {
				if c.Type == corev1.PodReady && c.Status == corev1.ConditionTrue {
					ready = true
				}
			}
		}
		if !ready {
			return fmt.Errorf("%s no esta Ready", comp)
		}
	}
	return nil
}

// Runtime sale de idle-escalado: sube a 1 réplica los componentes del warm.
type Runtime struct {
	C         kubernetes.Interface
	Namespace string
}

func (r *Runtime) ns(override ...string) string {
	if len(override) > 0 && override[0] != "" {
		return override[0]
	}
	if r.Namespace != "" {
		return r.Namespace
	}
	return wm.LegacyNamespace
}

// ScaleUp deja warm-app y warm-redis (Deployments) y warm-db (StatefulSet) con >= 1 réplica.
func (r *Runtime) ScaleUp(ctx context.Context) error {
	return r.ScaleUpInNamespace(ctx, r.ns())
}

// ScaleUpInNamespace escala los componentes en un namespace dado.
func (r *Runtime) ScaleUpInNamespace(ctx context.Context, ns string) error {
	if ns == "" {
		ns = r.ns()
	}
	one := int32(1)
	for _, n := range []string{"warm-app", "warm-redis"} {
		d, err := r.C.AppsV1().Deployments(ns).Get(ctx, n, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if d.Spec.Replicas == nil || *d.Spec.Replicas < 1 {
			d.Spec.Replicas = &one
			if _, err := r.C.AppsV1().Deployments(ns).Update(ctx, d, metav1.UpdateOptions{}); err != nil {
				return err
			}
		}
	}
	s, err := r.C.AppsV1().StatefulSets(ns).Get(ctx, "warm-db", metav1.GetOptions{})
	if err != nil {
		return err
	}
	if s.Spec.Replicas == nil || *s.Spec.Replicas < 1 {
		s.Spec.Replicas = &one
		_, err = r.C.AppsV1().StatefulSets(ns).Update(ctx, s, metav1.UpdateOptions{})
	}
	return err
}

// Deployer parchea la imagen de warm-app y consulta su rollout (deploy en proceso).
type Deployer struct {
	C         kubernetes.Interface
	Namespace string
}

func (d *Deployer) ns(override ...string) string {
	if len(override) > 0 && override[0] != "" {
		return override[0]
	}
	if d.Namespace != "" {
		return d.Namespace
	}
	return wm.LegacyNamespace
}

const (
	appName     = "warm-app"
	appSelector = "app.kubernetes.io/name=warm-app"
)

// SetImage cambia la imagen del contenedor warm-app con un strategic merge patch.
func (d *Deployer) SetImage(ctx context.Context, ref string) error {
	return d.SetImageInNamespace(ctx, d.ns(), ref)
}

// SetImageInNamespace cambia la imagen del contenedor en el namespace dado.
func (d *Deployer) SetImageInNamespace(ctx context.Context, ns, ref string) error {
	if ns == "" {
		ns = d.ns()
	}
	dep, err := d.C.AppsV1().Deployments(ns).Get(ctx, appName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	found := false
	for _, c := range dep.Spec.Template.Spec.Containers {
		found = found || c.Name == appName
	}
	if !found {
		return fmt.Errorf("el Deployment %s no tiene el contenedor %s", appName, appName)
	}
	patch, err := json.Marshal(map[string]any{"spec": map[string]any{"template": map[string]any{"spec": map[string]any{
		"containers": []any{map[string]any{"name": appName, "image": ref}}}}}})
	if err != nil {
		return err
	}
	_, err = d.C.AppsV1().Deployments(ns).Patch(ctx, appName, types.StrategicMergePatchType, patch, metav1.PatchOptions{})
	return err
}

// RolloutComplete exige el rollout completo de ref en el namespace por defecto.
func (d *Deployer) RolloutComplete(ctx context.Context, ref string) (bool, error) {
	return d.RolloutCompleteInNamespace(ctx, d.ns(), ref)
}

// RolloutCompleteInNamespace exige el rollout completo de ref en un namespace dado.
func (d *Deployer) RolloutCompleteInNamespace(ctx context.Context, ns, ref string) (bool, error) {
	if ns == "" {
		ns = d.ns()
	}
	dep, err := d.C.AppsV1().Deployments(ns).Get(ctx, appName, metav1.GetOptions{})
	if err != nil {
		return false, err
	}
	want := int32(1)
	if dep.Spec.Replicas != nil {
		want = *dep.Spec.Replicas
	}
	st := dep.Status
	if want == 0 || st.ObservedGeneration < dep.Generation || st.UpdatedReplicas != want || st.AvailableReplicas != want {
		return false, nil
	}
	if !hasImage(dep.Spec.Template.Spec.Containers, ref) {
		return false, nil
	}
	pl, err := d.C.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{LabelSelector: appSelector})
	if err != nil {
		return false, err
	}
	if int32(len(pl.Items)) != want {
		return false, nil
	}
	for _, p := range pl.Items {
		if p.DeletionTimestamp != nil || p.Status.Phase != corev1.PodRunning || !podReady(p) || !hasImage(p.Spec.Containers, ref) {
			return false, nil
		}
	}
	return true, nil
}

func hasImage(cs []corev1.Container, ref string) bool {
	for _, c := range cs {
		if c.Name == appName {
			return c.Image == ref
		}
	}
	return false
}

func podReady(p corev1.Pod) bool {
	for _, c := range p.Status.Conditions {
		if c.Type == corev1.PodReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}
