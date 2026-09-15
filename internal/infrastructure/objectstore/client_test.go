package objectstore_test

import (
	"context"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.uber.org/fx"

	"pet-messenger/internal/config"
	"pet-messenger/internal/infrastructure/objectstore"
)

func TestNewClient_CreatesBucketAndConnects(t *testing.T) {
	ctx := context.Background()
	req := testcontainers.ContainerRequest{
		Image:        "minio/minio:latest",
		ExposedPorts: []string{"9000/tcp"},
		Env:          map[string]string{"MINIO_ROOT_USER": "testuser", "MINIO_ROOT_PASSWORD": "testpassword"},
		Cmd:          []string{"server", "/data"},
		WaitingFor:   wait.ForHTTP("/minio/health/live").WithPort("9000/tcp"),
	}
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: req, Started: true})
	if err != nil {
		t.Fatalf("start minio container: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	endpoint, err := container.Endpoint(ctx, "")
	if err != nil {
		t.Fatalf("container endpoint: %v", err)
	}

	cfg := &config.Config{MinIO: config.MinIOConfig{
		Endpoint: endpoint, AccessKey: "testuser", SecretKey: "testpassword", Bucket: "avatars-test",
	}}

	client, err := objectstore.NewClient(fxNoopLifecycle{}, cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	exists, err := client.BucketExists(ctx, "avatars-test")
	if err != nil || !exists {
		t.Fatalf("bucket should exist after NewClient: exists=%v err=%v", exists, err)
	}
}

type fxNoopLifecycle struct{}

func (fxNoopLifecycle) Append(fx.Hook) {}
