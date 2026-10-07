package runner

import (
	"context"
	"io"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// maxLogBytes acota los logs copiados a la evidencia.
const maxLogBytes = 1 << 20

// KubeAPI es el puerto mínimo hacia Kubernetes para los runners.
type KubeAPI interface {
	CreateJob(ctx context.Context, j *batchv1.Job) error // AlreadyExists: apierrors.IsAlreadyExists
	ListJobs(ctx context.Context, selector string) ([]batchv1.Job, error)
	DeleteJob(ctx context.Context, name string) error // inexistente: sin error
	JobLogs(ctx context.Context, name string) ([]byte, error)
}

// ClientGo adapta kubernetes.Interface (real o fake) al puerto.
type ClientGo struct {
	CS kubernetes.Interface
	NS string
}

// CreateJob implementa KubeAPI.
func (c ClientGo) CreateJob(ctx context.Context, j *batchv1.Job) error {
	_, err := c.CS.BatchV1().Jobs(c.NS).Create(ctx, j, metav1.CreateOptions{})
	return err
}

// ListJobs implementa KubeAPI.
func (c ClientGo) ListJobs(ctx context.Context, selector string) ([]batchv1.Job, error) {
	l, err := c.CS.BatchV1().Jobs(c.NS).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, err
	}
	return l.Items, nil
}

// DeleteJob implementa KubeAPI (con sus pods en segundo plano).
func (c ClientGo) DeleteJob(ctx context.Context, name string) error {
	bg := metav1.DeletePropagationBackground
	err := c.CS.BatchV1().Jobs(c.NS).Delete(ctx, name, metav1.DeleteOptions{PropagationPolicy: &bg})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

// JobLogs implementa KubeAPI: logs del primer pod del Job (vacío si no hay pod).
func (c ClientGo) JobLogs(ctx context.Context, name string) ([]byte, error) {
	pods, err := c.CS.CoreV1().Pods(c.NS).List(ctx, metav1.ListOptions{LabelSelector: "job-name=" + name})
	if err != nil {
		return nil, err
	}
	if len(pods.Items) == 0 {
		return nil, nil
	}
	lim := int64(maxLogBytes)
	st, err := c.CS.CoreV1().Pods(c.NS).GetLogs(pods.Items[0].Name, &corev1.PodLogOptions{LimitBytes: &lim}).Stream(ctx)
	if err != nil {
		return nil, err
	}
	defer st.Close()
	return io.ReadAll(io.LimitReader(st, maxLogBytes))
}
