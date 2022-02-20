package docker

import (
	"context"
	"errors"
	"fmt"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/rs/zerolog/log"
)


type RunArgs struct {
	Image string
	MountDir string
	ExposedPort int
	Command []string
}

type Client struct {
	cli *client.Client
}

func New() (*Client, error) {
	cli, err  := client.NewEnvClient()
	if err != nil {
		return nil, fmt.Errorf("creating docker client: %w", err)
	}
	return &Client{
		cli,
	}, nil
}

func (c *Client) Run(ctx context.Context, args *RunArgs) error {
	log.Debug().Str("image", args.Image).Msg("pulling image")
	if err := c.pullImage(ctx, args.Image); err != nil {
		return fmt.Errorf("pulling image %s: %w", args.Image, err)
	}
	log.Debug().Msg("running container")
	if err := c.runContainer(ctx, args); err != nil {
		return fmt.Errorf("running container: %w", err)
	}
	return nil
}

func (c *Client) runContainer(ctx context.Context, args *RunArgs) error {
	// create the container
	config := &container.Config{
		Image: args.Image,
		Cmd: []string{
			"sleep",
			"86400",
		},
	}
	hostConfig := &container.HostConfig{}

	log.Debug().Msg("creating container")
	resp, err := c.cli.ContainerCreate(ctx, config, hostConfig, nil, nil, "test")
	if err != nil {
		return fmt.Errorf("creating container: %w", err)
	}

	// start the container
	log.Debug().Msg("starting container")
	if err := c.cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		return fmt.Errorf("starting container: %w", err)
	}

	// wait for the container to exit
	log.Debug().Msg("waiting for container")
	_, errC := c.cli.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	if err := <-errC; err != nil {
		if errors.Is(err, context.Canceled) {
			_ = c.cli.ContainerRemove(context.TODO(), resp.ID, types.ContainerRemoveOptions{
				Force: true,
			})
		} else {
			return fmt.Errorf("waiting for container: %w", err)
		}
	}
	return nil
}

func (c *Client) pullImage(ctx context.Context, imageName string) error {
	reader, err := c.cli.ImagePull(ctx, imageName, types.ImagePullOptions{})
	defer reader.Close()
	if err != nil {
		return fmt.Errorf("docker ImagePull: %w", err)
	}
	return nil
}
