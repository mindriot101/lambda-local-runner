package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/docker/docker/client"
	"github.com/mindriot101/lambda-local-runner/internal/docker"
	"github.com/mindriot101/lambda-local-runner/internal/server"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{
		Out: os.Stderr,
	})

	dockerClient, err := client.NewClientWithOpts(client.FromEnv)

	if err != nil {
		panic(err)
	}
	cli := docker.New(dockerClient)

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	imageName, err := cli.BuildImage(ctx)
	if err != nil {
		panic(err)
	}

	args := docker.RunContainerArgs{
		ContainerName: "test",
		ImageName:     imageName,
		Handler:       "app.lambda_handler",
		SourcePath:    "testproject/.aws-sam/build/HelloWorldFunction",
		Port:          9001,
	}

	go func() {
		if err := cli.RunContainer(ctx, args); err != nil {
			panic(err)
		}
	}()

	// run this each time the cancel func is not called
	srv := server.New()
	srv.AddRoute("GET", "/hello", 9001)
	srvCancel, err := srv.Run(8080)
	if err != nil {
		panic(err)
	}

	// host the container via web server
	select {
	case <-c:
		// cancel running docker containers
		cancel()

		// shutdown web server
		srvCancel()
	}
}
