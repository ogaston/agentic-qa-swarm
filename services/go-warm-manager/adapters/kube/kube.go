// Package kube contiene los adaptadores de go-warm-manager sobre client-go.
package kube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

// Jobs crea y consulta Jobs en el namespace de prueba.
type Jobs struct{ C kubernetes.Interface }

// Create convierte el Manifest en batchv1.Job (decodificación estricta) y lo crea.
func (j *Jobs) Create(ctx context.Context, m wm.Manifest) error {
	raw, err := json.Marshal(m)
	if err != nil {
		return err
	}
	var job batchv1.Job
	dec := json.NewDecoder(bytesReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&job); err != nil {
		return fmt.Errorf("manifiesto de Job invalido: %w", err)
	}
	if job.Namespace != wm.Namespace {
		return errors.New("el Job no es del namespace de prueba")
	}
	_, err = j.C.BatchV1().Jobs(wm.Namespace).Create(ctx, &job, metav1.CreateOptions{})
	return err
}

// Status resume las condiciones del Job.
func (j *Jobs) Status(ctx context.Context, name string) (wm.JobPhase, string, error) {
	job, err := j.C.BatchV1().Jobs(wm.Namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", "", err
	}
	for _, c := range job.Status.Conditions {
		if c.Status != corev1.ConditionTrue {
			continue
		}
		switch c.Type {
		case batchv1.JobComplete:
			return wm.JobSucceeded, "", nil
		case batchv1.JobFailed:
			return wm.JobFailed, c.Reason + ": " + c.Message, nil
		}
	}
	if job.Status.Succeeded > 0 {
		return wm.JobSucceeded, "", nil
	}
	return wm.JobPending, "", nil
}
