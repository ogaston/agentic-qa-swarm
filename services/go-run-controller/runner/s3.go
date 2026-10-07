package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// S3 es el adaptador de Evidence sobre S3/MinIO; el URI es s3://bucket/key.
type S3 struct {
	Client *minio.Client
	Bucket string
}

var _ Evidence = (*S3)(nil)

// NewS3 crea el cliente. endpoint es una URL http(s) (https => TLS). Las credenciales llegan por
// archivo (nunca por variable en claro): se leen aquí y no se guardan en ningún otro sitio.
func NewS3(endpoint, bucket, accessKeyFile, secretKeyFile string) (*S3, error) {
	u, err := url.Parse(endpoint)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || bucket == "" {
		return nil, errors.New("EVIDENCE_ENDPOINT debe ser http(s)://host[:puerto] y EVIDENCE_BUCKET no vacío")
	}
	ak, err := readSecretFile(accessKeyFile)
	if err != nil {
		return nil, err
	}
	sk, err := readSecretFile(secretKeyFile)
	if err != nil {
		return nil, err
	}
	c, err := minio.New(u.Host, &minio.Options{Creds: credentials.NewStaticV4(ak, sk, ""), Secure: u.Scheme == "https"})
	if err != nil {
		return nil, errors.New("cliente S3 inválido")
	}
	return &S3{Client: c, Bucket: bucket}, nil
}

func readSecretFile(p string) (string, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return "", errors.New("no se pudo leer el archivo de credenciales de evidencia")
	}
	s := strings.TrimSpace(string(b))
	if s == "" {
		return "", errors.New("archivo de credenciales de evidencia vacío")
	}
	return s, nil
}

func cleanKey(key string) (string, error) {
	k := path.Clean("/" + key)
	if k == "/" || strings.Contains(key, "..") {
		return "", errors.New("clave inválida")
	}
	return k[1:], nil
}

// URI implementa Evidence.
func (s *S3) URI(key string) string {
	return fmt.Sprintf("s3://%s/%s", s.Bucket, strings.TrimPrefix(key, "/"))
}

// Put implementa Evidence.
func (s *S3) Put(ctx context.Context, key string, data []byte) (string, error) {
	k, err := cleanKey(key)
	if err != nil {
		return "", err
	}
	if _, err := s.Client.PutObject(ctx, s.Bucket, k, bytes.NewReader(data), int64(len(data)),
		minio.PutObjectOptions{ContentType: "application/octet-stream"}); err != nil {
		return "", err
	}
	return s.URI(k), nil
}

// Get implementa Evidence (ErrNotFound solo para NoSuchKey).
func (s *S3) Get(ctx context.Context, key string) ([]byte, error) {
	k, err := cleanKey(key)
	if err != nil {
		return nil, err
	}
	o, err := s.Client.GetObject(ctx, s.Bucket, k, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer o.Close()
	b, err := io.ReadAll(o)
	if err != nil {
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return b, nil
}
