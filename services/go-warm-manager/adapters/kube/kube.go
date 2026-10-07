// Package kube contiene los adaptadores de go-warm-manager sobre client-go.
package kube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"

	wm "github.com/ogaston/agentic-qa-swarm/services/go-warm-manager"
)

// StateConfigMap es el ConfigMap que persiste el WarmState.
const StateConfigMap = "warm-state"

// StateStore guarda el WarmState en el ConfigMap warm-state del namespace de prueba.
type StateStore struct {
	C    kubernetes.Interface
	Seed wm.WarmState // se usa si el ConfigMap no existe
}

// Get lee el ConfigMap (o devuelve la semilla si no existe).
func (s *StateStore) Get(ctx context.Context) (wm.WarmState, error) {
	cm, err := s.C.CoreV1().ConfigMaps(wm.Namespace).Get(ctx, StateConfigMap, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return s.Seed, nil
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

// CompareAndSwap escribe next solo si el estado persistido es expect. Usa el resourceVersion del
// ConfigMap leido (Update condicional): un cambio concurrente da ErrStateConflict, nunca se pisa.
func (s *StateStore) CompareAndSwap(ctx context.Context, expect, next wm.WarmState) error {
	if !next.Valid() {
		return errors.New("WarmState invalido")
	}
	raw, _ := json.Marshal(next)
	api := s.C.CoreV1().ConfigMaps(wm.Namespace)
	cm, err := api.Get(ctx, StateConfigMap, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		if s.Seed != expect {
			return wm.ErrStateConflict
		}
		_, err = api.Create(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: StateConfigMap, Namespace: wm.Namespace},
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
	if !w.Valid() {
		return errors.New("WarmState invalido")
	}
	raw, _ := json.Marshal(w)
	api := s.C.CoreV1().ConfigMaps(wm.Namespace)
	cm, err := api.Get(ctx, StateConfigMap, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = api.Create(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: StateConfigMap, Namespace: wm.Namespace},
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

// Health considera sano el warm si los pods de app, DB y Redis están Ready.
type Health struct{ C kubernetes.Interface }

var warmComponents = []string{"warm-app", "warm-db", "warm-redis"}

// Check falla si falta algún pod Ready de los tres componentes.
func (h *Health) Check(ctx context.Context) error {
	for _, comp := range warmComponents {
		pods, err := h.C.CoreV1().Pods(wm.Namespace).List(ctx, metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=" + comp})
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
type Runtime struct{ C kubernetes.Interface }

// ScaleUp deja warm-app y warm-redis (Deployments) y warm-db (StatefulSet) con >= 1 réplica.
func (r *Runtime) ScaleUp(ctx context.Context) error {
	one := int32(1)
	for _, n := range []string{"warm-app", "warm-redis"} {
		d, err := r.C.AppsV1().Deployments(wm.Namespace).Get(ctx, n, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if d.Spec.Replicas == nil || *d.Spec.Replicas < 1 {
			d.Spec.Replicas = &one
			if _, err := r.C.AppsV1().Deployments(wm.Namespace).Update(ctx, d, metav1.UpdateOptions{}); err != nil {
				return err
			}
		}
	}
	s, err := r.C.AppsV1().StatefulSets(wm.Namespace).Get(ctx, "warm-db", metav1.GetOptions{})
	if err != nil {
		return err
	}
	if s.Spec.Replicas == nil || *s.Spec.Replicas < 1 {
		s.Spec.Replicas = &one
		_, err = r.C.AppsV1().StatefulSets(wm.Namespace).Update(ctx, s, metav1.UpdateOptions{})
	}
	return err
}

// Deployer parchea la imagen de warm-app y consulta su rollout (deploy en proceso).
type Deployer struct{ C kubernetes.Interface }

const (
	appName     = "warm-app"
	appSelector = "app.kubernetes.io/name=warm-app"
)

// SetImage cambia la imagen del contenedor warm-app con un strategic merge patch. Antes lee el
// Deployment y exige que el contenedor exista (un parche por nombre inexistente AGREGARIA un contenedor).
func (d *Deployer) SetImage(ctx context.Context, ref string) error {
	dep, err := d.C.AppsV1().Deployments(wm.Namespace).Get(ctx, appName, metav1.GetOptions{})
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
	_, err = d.C.AppsV1().Deployments(wm.Namespace).Patch(ctx, appName, types.StrategicMergePatchType, patch, metav1.PatchOptions{})
	return err
}

// RolloutComplete exige el rollout completo de ref (criterio copiado de AppReady de go-reset, mas la
// imagen): observedGeneration>=generation, updatedReplicas==replicas==availableReplicas, la plantilla y
// TODOS los pods con la imagen ref, pods reales exactamente los deseados, Running, Ready y ninguno
// terminando. Un pod viejo todavia Ready NO cuenta.
func (d *Deployer) RolloutComplete(ctx context.Context, ref string) (bool, error) {
	dep, err := d.C.AppsV1().Deployments(wm.Namespace).Get(ctx, appName, metav1.GetOptions{})
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
	pl, err := d.C.CoreV1().Pods(wm.Namespace).List(ctx, metav1.ListOptions{LabelSelector: appSelector})
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
