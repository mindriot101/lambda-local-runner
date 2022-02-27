package lambdahost

import (
	"context"
	"fmt"

	"github.com/mindriot101/lambda-local-runner/internal/docker"
	"github.com/rs/zerolog/log"
)

type dockerclient interface {
	RunContainer(ctx context.Context, args docker.RunContainerArgs) (string, error)
	RemoveContainer(ctx context.Context, containerID string) error
}

type LambdaHost struct {
	args        docker.RunContainerArgs
	events      chan instruction
	host        dockerclient
	containerID string
}

func New(client dockerclient, args docker.RunContainerArgs) *LambdaHost {
	return &LambdaHost{
		args:   args,
		host:   client,
		events: make(chan instruction, 10),
	}
}

func (h *LambdaHost) Shutdown() {
	h.send(instructionShutdown)
}

func (h *LambdaHost) Restart() {
	h.send(instructionRestart)
}

func (h *LambdaHost) send(ins instruction) {
	log.Debug().Interface("instruction", ins).Msg("sending instruction to host")
	h.events <- ins
}

func (h *LambdaHost) Run(ctx context.Context, done chan<- struct{}) error {
	if err := h.runContainer(ctx); err != nil {
		return fmt.Errorf("running containers: %w", err)
	}
	defer h.RemoveContainer(ctx)

	for ins := range h.events {
		logger := log.With().Interface("instruction", ins).Logger()
		logger.Debug().Msg("got message")

		switch ins {
		case instructionShutdown:
			logger.Debug().Msg("shutting down")
			done <- struct{}{}
			return nil

		case instructionRestart:
			logger.Debug().Msg("restarting")
			h.RemoveContainer(ctx)

			if err := h.runContainer(ctx); err != nil {
				return fmt.Errorf("running containers: %w", err)
			}

		default:
			log.Error().Interface("message_type", ins).Msg("invalid message received")
		}
	}

	return nil
}

func (h *LambdaHost) runContainer(ctx context.Context) error {
	var err error
	h.containerID, err = h.host.RunContainer(ctx, h.args)
	if err != nil {
		panic(err)
	}

	return nil
}

func (h *LambdaHost) RemoveContainer(ctx context.Context) error {
	return h.host.RemoveContainer(ctx, h.containerID)
}
