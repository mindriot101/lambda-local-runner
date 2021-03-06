package lambdahost

import (
	"context"
	"os"
	"sync"
	"testing"

	"github.com/mindriot101/lambda-local-runner/internal/docker"
	"github.com/rs/zerolog"
)

type call struct {
	name string
}

type mockClient struct {
	mu    sync.Mutex
	calls []call
}

func (m *mockClient) Calls() []call {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.calls
}

func (m *mockClient) RunContainer(ctx context.Context, args docker.RunContainerArgs) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.calls = append(m.calls, call{"RunContainer"})
	return "containerID", nil
}

func (m *mockClient) RemoveContainer(ctx context.Context, containerID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.calls = append(m.calls, call{"RemoveContainer"})
	return nil
}

func TestMain(m *testing.M) {
	zerolog.SetGlobalLevel(zerolog.WarnLevel)

	os.Exit(m.Run())
}

func TestShutdown(t *testing.T) {
	ctx := context.Background()
	args := docker.RunContainerArgs{}
	client := &mockClient{}
	host := New(client, args)
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go host.Run(ctx, done, &wg)
	host.Shutdown()
	<-done

	calls := client.Calls()
	nCalls := len(calls)
	if nCalls != 2 {
		t.Fatalf("invalid number of calls, found %d expected %d", nCalls, 2)
	}

	if calls[0].name != "RunContainer" {
		t.Fatalf("invalid call %s expected RunContainer", calls[0].name)
	}

	if calls[1].name != "RemoveContainer" {
		t.Fatalf("invalid call %s expected RemoveContainer", calls[1].name)
	}
}

func TestRestart(t *testing.T) {
	ctx := context.Background()
	args := docker.RunContainerArgs{}
	client := &mockClient{}
	host := New(client, args)
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go host.Run(ctx, done, &wg)
	host.Restart()
	host.Shutdown()
	<-done

	calls := client.Calls()
	nCalls := len(calls)
	if nCalls != 4 {
		t.Fatalf("invalid number of calls, found %d expected %d", nCalls, 4)
	}

	if calls[0].name != "RunContainer" {
		t.Fatalf("invalid call %s expected RunContainer", calls[0].name)
	}

	if calls[1].name != "RemoveContainer" {
		t.Fatalf("invalid call %s expected RemoveContainer", calls[1].name)
	}

	if calls[2].name != "RunContainer" {
		t.Fatalf("invalid call %s expected RunContainer", calls[2].name)
	}

	if calls[3].name != "RemoveContainer" {
		t.Fatalf("invalid call %s expected RemoveContainer", calls[3].name)
	}
}
