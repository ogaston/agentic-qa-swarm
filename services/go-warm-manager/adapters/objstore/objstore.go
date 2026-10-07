// Package objstore implementa warmmanager.ObjectStore sobre el sistema de archivos (pruebas) y S3/MinIO.
package objstore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// File guarda objetos bajo Dir; el URI es file://<ruta>.
type File struct{ Dir string }

func cleanKey(key string) (string, error) {
	k := path.Clean("/" + key)
	if k == "/" || strings.Contains(key, "..") {
		return "", errors.New("clave invalida")
	}
	return k[1:], nil
}

// Put escribe el objeto.
func (f *File) Put(_ context.Context, key string, data []byte) (string, error) {
	k, err := cleanKey(key)
	if err != nil {
		return "", err
	}
	p := filepath.Join(f.Dir, filepath.FromSlash(k))
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(p, data, 0o600); err != nil {
		return "", err
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	return "file://" + abs, nil
}

// Get lee un URI file://.
func (f *File) Get(_ context.Context, uri string) ([]byte, error) {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" {
		return nil, errors.New("uri no es file://")
	}
	return os.ReadFile(u.Path)
}

// S3 guarda objetos en un bucket S3/MinIO; el URI es s3://bucket/key.
type S3 struct {
	Client *minio.Client
	Bucket string
}

// NewS3 crea el cliente (endpoint host:puerto).
func NewS3(endpoint, access, secret, bucket string, secure bool) (*S3, error) {
	c, err := minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4(access, secret, ""), Secure: secure})
	if err != nil {
		return nil, err
	}
	return &S3{Client: c, Bucket: bucket}, nil
}

// Put sube el objeto y devuelve s3://bucket/key.
func (s *S3) Put(ctx context.Context, key string, data []byte) (string, error) {
	k, err := cleanKey(key)
	if err != nil {
		return "", err
	}
	_, err = s.Client.PutObject(ctx, s.Bucket, k, bytes.NewReader(data), int64(len(data)),
		minio.PutObjectOptions{ContentType: "application/json"})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("s3://%s/%s", s.Bucket, k), nil
}

// Get lee un URI s3:// del bucket configurado.
func (s *S3) Get(ctx context.Context, uri string) ([]byte, error) {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "s3" || u.Host != s.Bucket {
		return nil, errors.New("uri no es s3:// del bucket")
	}
	o, err := s.Client.GetObject(ctx, s.Bucket, strings.TrimPrefix(u.Path, "/"), minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer o.Close()
	return io.ReadAll(o)
}
