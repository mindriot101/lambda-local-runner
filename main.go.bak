package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/awslabs/goformation/v5"
	"github.com/awslabs/goformation/v5/cloudformation/serverless"
	"github.com/jessevdk/go-flags"
	"github.com/mindriot101/lambda-local-runner/internal/docker"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type Method string

const (
	MethodGET  Method = "GET"
	MethodPOST        = "POST"
)

type Endpoint struct {
	// URLPath is the path of the endpoint (not including host) e.g. `/foo`
	URLPath string
	// Method is the HTTP method used by the handler
	Method Method
}

// EndpointDefinition defines the information required per endpoint
// TODO: couples mulotiple concerns into one data structure
type HandlerDefinition struct {
	// LogicalID represents the name of the funciton in the cloudformation template
	LogicalID string
	// Architecture stores the architecture of the lambda (should be x86_64 if absent)
	Architecture string
	// Runtime stores the lambda runtime environment (e.g. python3.8, go1.x etc.)
	Runtime string
	// Handler is the name of the handler in a language-specific way
	Handler string
	// Port is the internal port of the listening container
	Port int
}

// EndpointMapping is a mapping from endpoint definition to the details needed to run the handler
// {
// 	(URLPath, Method): (LogicalID, Architecture, Runtime, Handler, Port),
// }
type EndpointMapping map[Endpoint]HandlerDefinition

func (e EndpointMapping) MarshalJSON() ([]byte, error) {
	out := make(map[string]HandlerDefinition)
	for k, v := range e {
		out[fmt.Sprintf("%s %s", strings.ToUpper(string(k.Method)), k.URLPath)] = v
	}

	res, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("marshalling endpoint mapping: %w", err)
	}
	return res, nil
}

func parseTemplate(filename string) (EndpointMapping, error) {
	template, err := goformation.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("parsing template: %w", err)
	}

	out := make(EndpointMapping)

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

			runtime := "x86_64"
			if f.Runtime != "" {
				runtime = f.Runtime
			}

			for _, event := range f.Events {
				if event.Type != "Api" {
					continue
				}

				// try to parse the ApiEvent
				evt := event.Properties.ApiEvent
				if evt.Method == "" || evt.Path == "" {
					continue
				}

				endpoint := Endpoint{
					URLPath: evt.Path,
					Method:  Method(evt.Method),
				}
				def := HandlerDefinition{
					LogicalID:    logicalID,
					Architecture: architecture,
					Runtime:      runtime,
					Handler:      f.Handler,
					Port:         -1,
				}
				out[endpoint] = def
			}

		default:
		}
	}

	return out, nil
}

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

	// parse template
	// get all lambda functions in the file
	// for each lambda function, get all events and build up a mapping
	mapping, err := parseTemplate(opts.Args.Template)
	if err != nil {
		return fmt.Errorf("parsing template: %w", err)
	}

	log.Debug().Interface("definition", mapping).Msg("parsed template")

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

	startPort := 9001
	var wg sync.WaitGroup
	for endpoint, definition := range mapping {
		wg.Add(1)
		// build a container with the desired runtime
		// run the container in a unique port in the background

		definition.Port = startPort
		startPort++

		imageName, err := cli.BuildImage(ctx, definition.Architecture)
		if err != nil {
			return fmt.Errorf("building image: %w", err)
		}
		log.Debug().Str("image_name", imageName).Msg("built image")
		sourcePath, _ := filepath.Abs(path.Join(opts.RootDir, definition.LogicalID))
		go func() {
			defer wg.Done()
			if err := cli.RunContainer(ctx, imageName, definition.Handler, sourcePath, definition.Port); err != nil {
				log.Fatal().Err(err).Msg("running container")
			}
		}()

		_ = endpoint
	}

	wg.Wait()

	return nil

	/*
		// zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
		// log.Logger = log.Output(zerolog.ConsoleWriter{
		// 	Out: os.Stderr,
		// })

		// s := server.New()
		// s.Add(lambdaenv.SpawnArgs{
		// 	Endpoint: "/",
		// })

		// ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		// defer cancel()
		// s.Serve(ctx)
		// return nil


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
	*/
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "%v", err)
		os.Exit(1)
	}
	os.Exit(0)
}
