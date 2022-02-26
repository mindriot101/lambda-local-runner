package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/docker/docker/client"
	"github.com/mindriot101/lambda-local-runner/internal/docker"
	"github.com/mindriot101/lambda-local-runner/internal/lambdahost"
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

	imageName, err := cli.BuildImage(context.TODO())
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

	host := lambdahost.New(cli, args)

	done := make(chan struct{})
	go host.Run(done)
	defer host.RemoveContainer()

	for {
		select {
		case <-c:
			log.Debug().Msg("got ctrl-c")
			host.Shutdown()
		case <-done:
			return
		}
	}
}
