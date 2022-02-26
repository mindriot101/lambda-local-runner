package docker

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/rs/zerolog/log"
)

const samVersion = "1.38.1"

// dockerclient represents the functions that we rely on from the docker API
type dockerclient interface {
	ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *specs.Platform, containerName string) (container.ContainerCreateCreatedBody, error)
	ContainerStart(ctx context.Context, containerID string, options types.ContainerStartOptions) error
	ContainerWait(context.Context, string, container.WaitCondition) (<-chan container.ContainerWaitOKBody, <-chan error)
	ContainerRemove(ctx context.Context, containerID string, options types.ContainerRemoveOptions) error
	ImageBuild(ctx context.Context, buildContext io.Reader, options types.ImageBuildOptions) (types.ImageBuildResponse, error)
	ContainerInspect(ctx context.Context, containerID string) (types.ContainerJSON, error)
}

type RunArgs struct {
	ExposedPort int
	Platform    string
	Handler     string
	SourcePath  string
}

type Client struct {
	cli dockerclient
}

func New(cli dockerclient) *Client {
	return &Client{
		cli,
	}
}

type RunContainerArgs struct {
	ContainerName string
	ImageName     string
	Handler       string
	SourcePath    string
	Port          int
}

func (c *Client) RunContainer(ctx context.Context, args RunContainerArgs) (string, error) {
	// create the container
	hPort := strconv.Itoa(args.Port)
	cPort := "8080"
	config := &container.Config{
		Image: args.ImageName,
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

	absSourcePath, _ := filepath.Abs(args.SourcePath)
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
				Source: absSourcePath,
				Target: "/var/task",
			},
		},
	}

	log.Debug().Msg("creating container")
	resp, err := c.cli.ContainerCreate(ctx, config, hostConfig, nil, nil, args.ContainerName)
	if err != nil {
		return "", fmt.Errorf("creating container: %w", err)
	}
	// start the container
	log.Debug().Str("container_id", resp.ID).Msg("starting container")
	if err := c.cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		return "", fmt.Errorf("starting container: %w", err)
	}

	log.Debug().Str("container_id", resp.ID).Msg("waiting for container to start")
	if err := c.containerWait(ctx, resp.ID); err != nil {
		return "", fmt.Errorf("waiting for container: %w", err)
	}

	return resp.ID, nil

	// wait for the container to exit
	// log.Debug().Msg("waiting for container")
	// resC, errC := c.cli.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)

	// // TODO: optionally print container logs

	// for {
	// 	select {
	// 	case msg := <-resC:
	// 		log.Debug().Interface("msg", msg).Msg("received message")
	// 	case err := <-errC:
	// 		if err != nil && !errors.Is(err, context.Canceled) {
	// 			return "", fmt.Errorf("waiting for container: %w", err)
	// 		}
	// 	}
	// }
}

func (c *Client) containerWait(ctx context.Context, containerID string) error {
	for {
		res, err := c.cli.ContainerInspect(ctx, containerID)
		if err != nil {
			return fmt.Errorf("waiting for container: %w", err)
		}
		switch res.State.Status {
		case "running":
			return nil
		case "removing", "exiting", "dead":
			return fmt.Errorf("container start failed with state %s", res.State.Status)
		default:
		}
	}
}

func (c *Client) RemoveContainer(ctx context.Context, containerID string) error {
	log.Debug().Str("container_id", containerID).Msg("removing container")
	if err := c.cli.ContainerRemove(ctx, containerID, types.ContainerRemoveOptions{
		Force: true,
	}); err != nil {
		return fmt.Errorf("removing container: %w", err)
	}
	return nil
}

// BuildImage builds a docker image
//
// https://stackoverflow.com/a/46518557
func (c *Client) BuildImage(ctx context.Context) (string, error) {
	// FIXME: hardcoding
	platform := "x86_64"
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

	imageName := fmt.Sprintf("lambda-local-runner-%s:latest", platform)
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
	_, _ = io.Copy(ioutil.Discard, res.Body)
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
