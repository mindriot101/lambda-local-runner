package docker

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strconv"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/rs/zerolog/log"
)

const samVersion = "1.38.1"

type RunArgs struct {
	MountDir    string
	ExposedPort int
	Platform    string
	Handler     string
}

type Client struct {
	cli *client.Client
}

func New() (*Client, error) {
	cli, err := client.NewEnvClient()
	if err != nil {
		return nil, fmt.Errorf("creating docker client: %w", err)
	}
	return &Client{
		cli,
	}, nil
}

func (c *Client) Run(ctx context.Context, args *RunArgs) error {
	imageName, err := c.buildImage(ctx, args.Platform)
	if err != nil {
		return fmt.Errorf("building image: %w", err)
	}
	log.Debug().Msg("running container")
	if err := c.runContainer(ctx, imageName, args); err != nil {
		return fmt.Errorf("running container: %w", err)
	}
	return nil
}

func (c *Client) runContainer(ctx context.Context, imageName string, args *RunArgs) error {
	// create the container
	hPort := strconv.Itoa(args.ExposedPort)
	cPort := "8080"
	config := &container.Config{
		Image: imageName,
		ExposedPorts: nat.PortSet{
			nat.Port(cPort): {},
		},
		Cmd: []string{"/var/aws-lambda-rie", "--log-level", "debug"},
		Env: []string{
			"AWS_LAMBDA_FUNCTION_VERSION=$LATEST",
			fmt.Sprintf("AWS_LAMBDA_FUNCTION_NAME=%s", args.Handler),
			"AWS_LAMBDA_FUNCTION_MEMORY_SIZE=128",
		},
	}

	hostConfig := &container.HostConfig{
		PortBindings: nat.PortMap{
			nat.Port(cPort): []nat.PortBinding{
				{
					HostIP:   "0.0.0.0",
					HostPort: hPort,
				},
			},
		},
	}

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
	resC, errC := c.cli.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	for {
		select {
		case msg := <-resC:
			log.Debug().Interface("msg", msg).Msg("received message")
			return nil
		case err := <-errC:
			if err == nil {
				continue
			}

			if errors.Is(err, context.Canceled) {
				log.Debug().Msg("got context cancellation, removing container")
				err = c.cli.ContainerRemove(context.TODO(), resp.ID, types.ContainerRemoveOptions{
					Force: true,
				})
				if err != nil {
					log.Warn().Err(err).Msg("error removing container")
				}
				return nil
			} else {
				return fmt.Errorf("waiting for container: %w", err)
			}
		}
	}
	// if err := <-errC; err != nil {
	// 	if errors.Is(err, context.Canceled) {
	// 		log.Debug().Msg("got context cancellation, removing container")
	// 		err = c.cli.ContainerRemove(context.TODO(), resp.ID, types.ContainerRemoveOptions{
	// 			Force: true,
	// 		})
	// 		if err != nil {
	// 			log.Warn().Err(err).Msg("error removing container")
	// 		}
	// 	} else {
	// 		return fmt.Errorf("waiting for container: %w", err)
	// 	}
	// }
	return nil
}

// buildImage builds a docker image
//
// https://stackoverflow.com/a/46518557
func (c *Client) buildImage(ctx context.Context, platform string) (string, error) {
	riePath, err := fetchRIE(platform)
	if err != nil {
		return "", fmt.Errorf("fetching lambda RIE: %w", err)
	}
	// FIXME: hardcoding runtime
	dockerfileSrc := fmt.Sprintf(`
FROM  public.ecr.aws/sam/emulation-python3.8:latest

COPY aws-lambda-rie /var/aws-lambda-rie
	`)

	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)
	defer tw.Close()

	// write dockerfile
	writeTarEntry(tw, "Dockerfile", []byte(dockerfileSrc), 0o655)

	// write aws-lambda-rie
	lambdaRIESrc, err := ioutil.ReadFile(riePath)
	if err != nil {
		return "", fmt.Errorf("reading rie contents: %w", err)
	}
	writeTarEntry(tw, "aws-lambda-rie", lambdaRIESrc, 0o777)

	buildContext := bytes.NewReader(buf.Bytes())

	imageName := "lambda-local-runner:latest"
	res, err := c.cli.ImageBuild(ctx, buildContext, types.ImageBuildOptions{
		Tags:       []string{imageName},
		Context:    buildContext,
		Dockerfile: "Dockerfile",
		Remove:     true,
		PullParent: true,
	})
	if err != nil {
		return "", fmt.Errorf("building image: %w", err)
	}
	defer res.Body.Close()
	_, _ = io.Copy(os.Stdout, res.Body)
	if err != nil {
		return "", fmt.Errorf("printing build command output: %w", err)
	}

	return imageName, nil
}

func fetchRIE(platform string) (string, error) {
	cacheLocation := fmt.Sprintf("/tmp/aws-lambda-rie-%s", platform)
	info, err := os.Stat(cacheLocation)
	if err != nil {
		// TODO: FETCH
	}
	if info.IsDir() {
		// TODO: remove directory
	}
	return cacheLocation, nil
}

func writeTarEntry(tarfile *tar.Writer, name string, contents []byte, mode int64) error {
	if err := tarfile.WriteHeader(&tar.Header{
		Name: name,
		Size: int64(len(contents)),
		Mode: mode,
	}); err != nil {
		return fmt.Errorf("writing tar header: %w", err)
	}
	if _, err := tarfile.Write([]byte(contents)); err != nil {
		log.Printf("writing file contents: %w", err)
	}
	return nil
}
