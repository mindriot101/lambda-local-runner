package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/jessevdk/go-flags"
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

	var opts struct {
		Verbose []bool `short:"v" long:"verbose" description:"Print verbose logging output"`
		// Args      struct {
		// 	Name string `required:"yes" positional-arg-name:"stack-name"`
		// } `positional-args:"yes" required:"yes"`
	}

	_, err := flags.Parse(&opts)
	if err != nil {
		os.Exit(1)
	}

	switch len(opts.Verbose) {
	case 0:
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	case 1:
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	default:
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}

	log.Debug().Msg("creating docker client")
	cli, err := docker.New()
	if err != nil {
		log.Fatal().Err(err).Msg("")
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

	done := make(chan struct{})
	go func() {
		log.Debug().Msg("spawning container")
		if err = env.Spawn(ctx, lambdaenv.SpawnArgs{
			Runtime:      "python3.8",
			Architecture: "x86_64",
			Handler:      "app.lambda_handler",
		}); err != nil {
			log.Fatal().Err(err).Msg("")
		}

		done <- struct{}{}
	}()

	select {
	case <-done:
		return
	}
}
