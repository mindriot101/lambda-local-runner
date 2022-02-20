package main

import (
	"context"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

func main() {
	cli, err := client.NewEnvClient()
	if err != nil {
		panic(err)
	}

	ctx := context.Background()

	// create the container
	config := &container.Config{
		Image:        "ubuntu:latest",
		ExposedPorts: nat.PortSet{"8080": struct{}{}},
		Cmd: []string{
			"sleep",
			"86400",
		},
	}
	hostConfig := &container.HostConfig{
		PortBindings: map[nat.Port][]nat.PortBinding{
			nat.Port("8080"): {{
				HostIP:   "127.0.0.1",
				HostPort: "8080",
			}},
		},
	}

	resp, err := cli.ContainerCreate(ctx, config, hostConfig, nil, nil, "test")
	if err != nil {
		panic(err)
	}

	if err := cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		panic(err)
	}
}
