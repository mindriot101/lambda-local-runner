package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/mindriot101/lambda-local-runner/internal/docker"
	"github.com/mindriot101/lambda-local-runner/internal/lambdaenv"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{
		Out: os.Stderr,
	})

	// TODO command line arguments

	zerolog.SetGlobalLevel(zerolog.DebugLevel)

	log.Debug().Msg("creating docker client")
	cli, err := docker.New()
	if err != nil {
		panic(err)
	}
	log.Debug().Msg("creating lambda client")
	env := lambdaenv.New(cli)

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

	// spawn in the foreground for now
	log.Debug().Msg("spawning container")
	if err = env.Spawn(ctx, lambdaenv.SpawnArgs{
		Runtime:      "python3.8",
		Architecture: "x86_64",
		Handler:      "app.lambda_handler",
	}); err != nil {
		panic(err)
	}
}
