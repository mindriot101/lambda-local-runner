package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path"
	"sync"

	"github.com/awslabs/goformation/v5"
	"github.com/awslabs/goformation/v5/cloudformation/serverless"
	"github.com/jessevdk/go-flags"
	"github.com/mindriot101/lambda-local-runner/internal/docker"
	"github.com/mindriot101/lambda-local-runner/internal/lambdaenv"
	"github.com/mindriot101/lambda-local-runner/internal/server"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func run() error {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{
		Out: os.Stderr,
	})

	var opts struct {
		Verbose []bool `short:"v" long:"verbose" description:"Print verbose logging output"`
		RootDir string `short:"r" long:"root" description:"Unpacked root directory" required:"yes"`
		Args    struct {
			Template string `required:"yes" positional-arg-name:"template"`
		} `positional-args:"yes" required:"yes"`
	}

	_, err := flags.Parse(&opts)
	if err != nil {
		return nil
	}

	switch len(opts.Verbose) {
	case 0:
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	case 1:
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	default:
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}

	functions, err := parseTemplate(opts.Args.Template)
	if err != nil {
		panic(err)
	}

	log.Debug().Msg("creating docker client")
	cli, err := docker.New()
	if err != nil {
		return fmt.Errorf("creating docker client: %w", err)
	}

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

	svr := server.New()
	var wg sync.WaitGroup
	for _, function := range functions {
		wg.Add(1)
		log.Debug().Msg("creating lambda client")

		functionDir := path.Join(opts.RootDir, function.Name)
		log.Debug().Str("functionpath", functionDir).Msg("loading function")

		svr.Add(function)

		go func(fn lambdaenv.SpawnArgs) {
			defer wg.Done()
			env := lambdaenv.New(cli, functionDir)
			log.Debug().Msg("spawning container")
			if err = env.Spawn(ctx, fn); err != nil {
				log.Fatal().Err(err).Msg("")
			}
		}(function)
	}

	go func() {
		if err := svr.Serve(ctx); err != nil {
			log.Fatal().Err(err).Msg("")
		}
	}()

	wg.Wait()
	return nil
}

func parseTemplate(filename string) ([]lambdaenv.SpawnArgs, error) {
	template, err := goformation.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("parsing template: %w", err)
	}

	functions := []lambdaenv.SpawnArgs{}
	for logicalID, resource := range template.Resources {
		switch resource.AWSCloudFormationType() {
		case "AWS::Serverless::Function":
			f, ok := resource.(*serverless.Function)
			if !ok {
				return nil, fmt.Errorf("invalid function %s", logicalID)
			}

			var architecture string
			if len(f.Architectures) >= 1 {
				architecture = f.Architectures[0]
			} else {
				architecture = "x86_64"
			}

			for _, event := range f.Events {
				if event.Type != "Api" {
					continue
				}

				// try to parse the ApiEvent
				evt := event.Properties.ApiEvent
				if evt.Method == "" {
					continue
				}

				// we have an api event, so roll with it
				args := lambdaenv.SpawnArgs{
					Name:         logicalID,
					Architecture: architecture,
					Runtime:      f.Runtime,
					Handler:      f.Handler,
					Endpoint:     evt.Path,
					Method:       lambdaenv.Method(evt.Method),
				}
				functions = append(functions, args)
				// TODO: only select the first valid event
				break
			}

		default:
		}
	}

	return functions, nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "%v", err)
		os.Exit(1)
	}
	os.Exit(0)
}
