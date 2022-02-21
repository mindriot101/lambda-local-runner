package docker

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/rs/zerolog/log"
)

const samVersion = "1.38.1"

type RunArgs struct {
	ExposedPort int
	Platform    string
	Handler     string
	SourcePath  string
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
			fmt.Sprintf("AWS_LAMBDA_FUNCTION_HANDLER=%s", args.Handler),
			"AWS_LAMBDA_FUNCTION_NAME=HelloWorldFunction",
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
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeBind,
				Source: args.SourcePath,
				Target: "/var/task",
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

	// get the container output
	outR, err := c.cli.ContainerLogs(ctx, resp.ID, types.ContainerLogsOptions{
		ShowStdout: true,
		Follow:     true,
	})
	if err != nil {
		log.Fatal().Err(err).Msg("getting container logs")
	}
	defer outR.Close()
	_, _ = io.Copy(os.Stderr, outR)

	// outC := make(chan []byte, 10)
	// go func() {
	// 	log.Debug().Msg("getting container logs")
	// 	outR, err := c.cli.ContainerLogs(ctx, resp.ID, types.ContainerLogsOptions{
	// 		ShowStdout: true,
	// 		Follow:     true,
	// 	})
	// 	log.Debug().Msg("container logs command returned")
	// 	if err != nil {
	// 		log.Warn().Err(err).Msg("getting container logs")
	// 		return
	// 	}
	// 	defer outR.Close()

	// 	buf := [80]byte{}
	// 	for {
	// 		log.Debug().Msg("fetching more bytes from container log output")
	// 		n, err := outR.Read(buf[:])
	// 		if err != nil {
	// 			log.Warn().Err(err).Msg("reading from container output")
	// 		}
	// 		if n == 0 {
	// 			break
	// 		}
	// 		outC <- buf[:]
	// 	}
	// }()

	for {
		select {
		// case msg := <-outC:
		// 	log.Debug().Interface("msg", msg).Msg("log")
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

const rieAMD64Url = "https://github.com/aws/aws-lambda-runtime-interface-emulator/releases/latest/download/aws-lambda-rie"
const rieARM64Url = "https://github.com/aws/aws-lambda-runtime-interface-emulator/releases/latest/download/aws-lambda-rie-arm64"

func fetchRIE(platform string) (string, error) {
	var url string
	switch platform {
	case "x86_64":
		url = rieAMD64Url
	case "arm64":
		url = rieARM64Url
	default:
		return "", fmt.Errorf("unsupported platform %s", platform)
	}
	cacheLocation := fmt.Sprintf("/tmp/aws-lambda-rie-%s", platform)

	logger := log.With().Str("src", url).Str("dest", cacheLocation).Logger()

	logger.Debug().Msg("fetching file")

	info, err := os.Stat(cacheLocation)
	if err == nil {
		logger.Debug().Msg("entry exists")
		if !info.IsDir() {
			logger.Debug().Msg("cached file found")
			return cacheLocation, nil
		}

		return "", fmt.Errorf("cahce location %s is a directory", cacheLocation)
	}

	// file does not exist so fetch
	logger.Debug().Msg("fetching remote file")
	if err = fetchFile(cacheLocation, rieAMD64Url); err != nil {
		return "", fmt.Errorf("fetching file: %w", err)
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
		return fmt.Errorf("writing file contents: %w", err)
	}
	return nil
}

func fetchFile(filepath, url string) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("making http request: %w", err)
	}
	defer resp.Body.Close()

	out, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("creating filepath %s: %w", filepath, err)
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return fmt.Errorf("copying file contents: %w", err)
	}
	return nil
}
