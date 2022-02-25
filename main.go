package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/docker/docker/client"
	"github.com/mindriot101/lambda-local-runner/internal/docker"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{
		Out: os.Stderr,
	})

	dockerClient, err := client.NewEnvClient()

	if err != nil {
		panic(err)
	}
	cli := docker.New(dockerClient)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	go func() {
		for range c {
			log.Debug().Msg("got ctrl-c event")
			cancel()
		}
	}()

	imageName, err := cli.BuildImage(ctx)
	if err != nil {
		panic(err)
	}

	if err := cli.RunContainer(ctx, "test", imageName, "app.lambda_handler", "testproject/.aws-sam/build/HelloWorldFunction", 9001); err != nil {
		panic(err)
	}
}
