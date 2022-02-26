package lambdahost

import (
	"context"
	"testing"

	"github.com/mindriot101/lambda-local-runner/internal/docker"
)

type call struct {
	name string
}


type mockClient struct {
	calls []call
}

func (m *mockClient) RunContainer(ctx context.Context, args docker.RunContainerArgs) (string, error) {
	m.calls = append(m.calls, call{"RunContainer"})
	return "containerID", nil
}

func (m *mockClient) RemoveContainer(ctx context.Context, containerID string) error {
	m.calls = append(m.calls, call{"RemoveContainer"})
	return nil
}

func TestShutdown(t *testing.T) {
	args := docker.RunContainerArgs{}
	client := &mockClient{}
	host := New(client, args)
	done := make(chan struct{})
	go host.Run(done)
	host.Shutdown()
	<-done

	nCalls := len(client.calls)
	if nCalls != 2 {
		t.Fatalf("invalid number of calls, found %d expected %d", nCalls, 2)
	}

	if client.calls[0].name != "RunContainer" {
		t.Fatalf("invalid call %s expected RunContainer", client.calls[0].name)
	}

	if client.calls[1].name != "RemoveContainer" {
		t.Fatalf("invalid call %s expected RemoveContainer", client.calls[1].name)
	}
}

func TestRestart(t *testing.T) {
	args := docker.RunContainerArgs{}
	client := &mockClient{}
	host := New(client, args)
	done := make(chan struct{})
	go host.Run(done)
	host.Restart()
	host.Shutdown()
	<-done

	nCalls := len(client.calls)
	if nCalls != 4 {
		t.Fatalf("invalid number of calls, found %d expected %d", nCalls, 4)
	}

	if client.calls[0].name != "RunContainer" {
		t.Fatalf("invalid call %s expected RunContainer", client.calls[0].name)
	}

	if client.calls[1].name != "RemoveContainer" {
		t.Fatalf("invalid call %s expected RemoveContainer", client.calls[1].name)
	}

	if client.calls[2].name != "RunContainer" {
		t.Fatalf("invalid call %s expected RunContainer", client.calls[2].name)
	}

	if client.calls[3].name != "RemoveContainer" {
		t.Fatalf("invalid call %s expected RemoveContainer", client.calls[3].name)
	}
}