package docker

import (
	"context"
	"io"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
)

type recordingClient struct {
	createFn func(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkConfig *network.NetworkingConfig, platform *specs.Platform, containerName string) (container.ContainerCreateCreatedBody, error)
	startFn  func(ctx context.Context, containerID string, options types.ContainerStartOptions) error
	waitFn   func(context.Context, string, container.WaitCondition) (<-chan container.ContainerWaitOKBody, <-chan error)
	removeFn func(ctx context.Context, containerID string, options types.ContainerRemoveOptions) error
	buildFn  func(ctx context.Context, buildContext io.Reader, options types.ImageBuildOptions) (types.ImageBuildResponse, error)
}

func (r *recordingClient) ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkConfig *network.NetworkingConfig, platform *specs.Platform, containerName string) (container.ContainerCreateCreatedBody, error) {
	if r.createFn != nil {
		return r.createFn(ctx, config, hostConfig, networkConfig, platform, containerName)
	}
	return container.ContainerCreateCreatedBody{}, nil
}

func (r *recordingClient) ContainerStart(ctx context.Context, containerID string, options types.ContainerStartOptions) error {
	if r.startFn != nil {
		return r.startFn(ctx, containerID, options)
	}
	return nil
}

func (r *recordingClient) ContainerWait(ctx context.Context, containerID string, condition container.WaitCondition) (<-chan container.ContainerWaitOKBody, <-chan error) {
	if r.waitFn != nil {
		return r.waitFn(ctx, containerID, condition)
	}
	return nil, nil
}
func (r *recordingClient) ContainerRemove(ctx context.Context, containerID string, options types.ContainerRemoveOptions) error {
	if r.removeFn != nil {
		return r.removeFn(ctx, containerID, options)
	}
	return nil
}
func (r *recordingClient) ImageBuild(ctx context.Context, buildContext io.Reader, options types.ImageBuildOptions) (types.ImageBuildResponse, error) {
	if r.buildFn != nil {
		return r.buildFn(ctx, buildContext, options)
	}
	return types.ImageBuildResponse{}, nil
}

var _ dockerclient = (*recordingClient)(nil)

// call records all call arguments to ensure that they are correct
type call struct {
	args []interface{}
}

/*
func TestCreateImage(t *testing.T) {
	c := recordingClient{}

	calls := []call{}

	c.createFn = func(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkConfig *network.NetworkingConfig, platform *specs.Platform, containerName string) (container.ContainerCreateCreatedBody, error) {
		calls = append(calls, call{
			args: []interface{}{
				config, hostConfig, networkConfig, platform, containerName,
			},
		})
		return container.ContainerCreateCreatedBody{}, nil
	}

	c.startFn = func(ctx context.Context, containerID string, options types.ContainerStartOptions) error {
		return nil
	}

	resC := make(chan container.ContainerWaitOKBody, 1)
	c.waitFn = func(context.Context, string, container.WaitCondition) (<-chan container.ContainerWaitOKBody, <-chan error) {
		resC <- container.ContainerWaitOKBody{}
		return resC, make(chan error, 1)
	}

	ctx := context.Background()
	d := New(&c)
	if err := d.RunContainer(ctx, "image-name", "handler", "sourcepath", 10001); err != nil {
		t.Fatalf("error running container: %v", err)
	}

	if len(calls) != 1 {
		t.Fatalf("invalid number of calls, expected 1 got %d", len(calls))
	}

	call := calls[0]
	if call.args[4].(string) != "image-name" {
		t.Fatalf("invalid image name, expected image-name, got %s", call.args[4].(string))
	}
}
*/
