package lambdaenv

import (
	"context"
	"testing"

	"github.com/mindriot101/lambda-local-runner/internal/docker"
)

type mock struct {
}

func (m *mock) Run(ctx context.Context, args *docker.RunArgs) error {
	return nil
}

func TestAssignPort(t *testing.T) {
	api := &mock{}
	env := New(api)

	port := env.newPort()
	if port != 9001 {
		t.Fatalf("invalid port, expected 9001, got %d", port)
	}
	port = env.newPort()
	if port != 9002 {
		t.Fatalf("invalid port, expected 9002, got %d", port)
	}
}
