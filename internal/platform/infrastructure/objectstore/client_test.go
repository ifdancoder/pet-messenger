package objectstore_test

import (
	"context"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.uber.org/fx"

	"pet-messenger/internal/platform/config"
	"pet-messenger/internal/platform/infrastructure/objectstore"
)

func startMinioContainer(t *testing.T) *config.Config {
	t.Helper()
	ctx := context.Background()
	req := testcontainers.ContainerRequest{
		// minio/minio was pulled from Docker Hub in September 2026, and
		// quay.io/minio/minio stopped allowing anonymous pulls three weeks
		// after that; cgr.dev/chainguard/minio is Chainguard's
		// free-to-pull community mirror, verified against the same
		// Cmd/Env/WaitingFor shape this test already uses.
		Image:        "cgr.dev/chainguard/minio:latest",
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
	waitUntilReady(t, endpoint)
	return &config.Config{MinIO: config.MinIOConfig{Endpoint: endpoint, AccessKey: "testuser", SecretKey: "testpassword", Bucket: "avatars-test"}}
}

// waitUntilReady closes a real gap between the health endpoint answering
// and the erasure-set backend actually being ready to serve S3 calls:
// under load, ListBuckets can still fail with "Server not initialized yet"
// for a short while after /minio/health/live already returned 200 (that
// gap is what produced "minio bucket exists avatars-test: Server not
// initialized yet, please try again" intermittently).
func waitUntilReady(t *testing.T, endpoint string) {
	t.Helper()
	client, err := minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4("testuser", "testpassword", "")})
	if err != nil {
		t.Fatalf("probe client: %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		if _, lastErr = client.ListBuckets(context.Background()); lastErr == nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("minio never became ready to serve S3 calls: %v", lastErr)
}

func TestNewClient_CreatesBucketAndConnects(t *testing.T) {
	cfg := startMinioContainer(t)

	client, err := objectstore.NewClient(fxNoopLifecycle{}, cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	exists, err := client.BucketExists(context.Background(), cfg.MinIO.Bucket)
	if err != nil || !exists {
		t.Fatalf("bucket should exist after NewClient: exists=%v err=%v", exists, err)
	}
}

type fxNoopLifecycle struct{}

func (fxNoopLifecycle) Append(fx.Hook) {}
