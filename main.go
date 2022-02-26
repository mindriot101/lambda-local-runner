package main

import (
	"context"
	"fmt"
	"os"

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

	dockerClient, err := client.NewClientWithOpts(client.FromEnv)

	if err != nil {
		panic(err)
	}
	cli := docker.New(dockerClient)

	// c := make(chan os.Signal, 1)
	// signal.Notify(c, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	// ctx, cancel := context.WithCancel(context.Background())
	// defer cancel()

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

	events := make(chan instruction, 1)
	srv := LambdaHost{
		args:   args,
		events: events,
		host:   cli,
	}

	done := make(chan struct{})
	go srv.Run(done)
	defer srv.removeContainer()

	// log.Debug().Msg("sleeping for 5 seconds")
	// time.Sleep(5 * time.Second)
	log.Debug().Msg("restarting containers")
	srv.Send(instruction{
		typ: instructionRestart,
	})
	// log.Debug().Msg("sleeping for 5 seconds")
	// time.Sleep(5 * time.Second)
	log.Debug().Msg("sending shutdown")
	srv.Send(instruction{
		typ: instructionShutdown,
	})
	log.Debug().Msg("done")
	<-done
}

type instructionType int

const (
	instructionShutdown instructionType = iota
	instructionRestart
)

type instruction struct {
	typ instructionType
}

type LambdaHost struct {
	args        docker.RunContainerArgs
	events      chan instruction
	host        *docker.Client
	containerID string
}

func (h *LambdaHost) Send(ins instruction) {
	h.events <- ins
}

func (h *LambdaHost) Run(done chan<- struct{}) error {
	if err := h.runContainer(); err != nil {
		return fmt.Errorf("running containers: %w", err)
	}
	defer h.removeContainer()

	for ins := range h.events {
		switch ins.typ {
		case instructionShutdown:
			done <- struct{}{}
			return nil

		case instructionRestart:
			h.removeContainer()

			if err := h.runContainer(); err != nil {
				return fmt.Errorf("running containers: %w", err)
			}

		default:
			log.Error().Interface("message_type", ins.typ).Msg("invalid message received")
		}
	}

	return nil
}

func (h *LambdaHost) runContainer() error {
	var err error
	h.containerID, err = h.host.RunContainer(context.TODO(), h.args)
	if err != nil {
		panic(err)
	}

	return nil
}

func (h *LambdaHost) removeContainer() error {
	return h.host.RemoveContainer(context.TODO(), h.containerID)
}
