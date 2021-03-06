package lambdahost

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/mindriot101/lambda-local-runner/internal/docker"
	"github.com/rs/zerolog/log"
)

type dockerclient interface {
	RunContainer(ctx context.Context, args docker.RunContainerArgs) (string, error)
	RemoveContainer(ctx context.Context, containerID string) error
}

type LambdaHost struct {
	args   docker.RunContainerArgs
	events chan instruction
	host   dockerclient

	mu          sync.Mutex
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

func (h *LambdaHost) Run(ctx context.Context, done chan<- struct{}, runWg *sync.WaitGroup) error {
	if err := h.runContainer(ctx); err != nil {
		return fmt.Errorf("running containers: %w", err)
	}
	runWg.Done()

	for ins := range h.events {
		logger := log.With().Interface("instruction", ins).Logger()
		logger.Debug().Msg("got message")

		switch ins {
		case instructionShutdown:
			logger.Debug().Msg("shutting down")
			if err := h.RemoveContainer(context.TODO()); err != nil {
				logger.
					Warn().
					Err(err).
					Str("container_name", h.args.ContainerName).
					Msg("could not remove the lambda container")
			}
			done <- struct{}{}
			return nil

		case instructionRestart:
			logger.Debug().Msg("restarting")
			if err := h.RemoveContainer(context.TODO()); err != nil {
				logger.
					Warn().
					Err(err).
					Str("container_name", h.args.ContainerName).
					Msg("could not remove the lambda container")
			}

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
	h.mu.Lock()
	defer h.mu.Unlock()

	h.containerID, err = h.host.RunContainer(ctx, h.args)
	if err != nil {
		return fmt.Errorf("running container: %w", err)
	}

	return nil
}

func (h *LambdaHost) RemoveContainer(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return h.host.RemoveContainer(ctx, h.containerID)
}
