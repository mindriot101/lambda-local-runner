package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"path"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/awslabs/goformation/v6"
	"github.com/awslabs/goformation/v6/cloudformation/serverless"
	"github.com/docker/docker/client"
	"github.com/fsnotify/fsnotify"
	"github.com/jessevdk/go-flags"
	"github.com/mindriot101/lambda-local-runner/internal/docker"
	"github.com/mindriot101/lambda-local-runner/internal/lambdahost"
	"github.com/mindriot101/lambda-local-runner/internal/server"
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

// https://stackoverflow.com/a/31832326
var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

// https://stackoverflow.com/a/31832326
func randStringRunes(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

// containerName generates the container name that is meant to be both:
// * informative, and
// * unique
func containerName(endpoint Endpoint, definition HandlerDefinition) string {
	sanitisedURL := strings.ReplaceAll(endpoint.URLPath, "/", "_")
	return fmt.Sprintf("llr-%s-%s%s-%s", definition.LogicalID, endpoint.Method, sanitisedURL, randStringRunes(6))
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
			if len(*f.Architectures) >= 1 {
				architecture = (*f.Architectures)[0]
			} else {
				architecture = "x86_64"
			}

			runtime := "x86_64"
			if *f.Runtime != "" {
				runtime = *f.Runtime
			}

			for _, event := range *f.Events {
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
					Handler:      *f.Handler,
					Port:         -1,
				}
				out[endpoint] = def
			}

		default:
		}
	}

	return out, nil
}

type Args struct {
	Template string `required:"yes" positional-arg-name:"template"`
}

type Opts struct {
	Verbose []bool `short:"v" long:"verbose" description:"Print verbose logging output"`
	RootDir string `short:"r" long:"root"    description:"Unpacked root directory"      required:"yes"`
	Port    int    `short:"p" long:"port"    description:"Server port to listen on"                    default:"8080"`
	Host    string `short:"H" long:"host"    description:"Host to listen on"                           default:"localhost"`
	Args    Args   `                                                                    required:"yes"                     positional-args:"yes"`
}

func run(ctx context.Context, opts Opts) error {
	endpointMapping, err := parseTemplate(opts.Args.Template)
	if err != nil {
		return fmt.Errorf("parsing template: %w", err)
	}
	log.Debug().Interface("endpoint_mapping", endpointMapping).Msg("parsed template")

	dockerClient, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return fmt.Errorf("connecting to docker: %w", err)
	}
	cli := docker.New(dockerClient)

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	srv := server.New(opts.Host, opts.Port)
	containerIdx := 0
	containerPort := 9001
	lambdaHosts := []*lambdahost.LambdaHost{}
	done := make(chan struct{})
	dockerCtx := context.Background()

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("creating file system watcher: %w", err)
	}
	defer watcher.Close()

	endpointStrings := []string{}
	var wg sync.WaitGroup
	for endpoint, definition := range endpointMapping {
		wg.Add(1)

		imageName, err := cli.BuildImage(dockerCtx)
		if err != nil {
			return fmt.Errorf("building docker image: %w", err)
		}

		containerName := containerName(endpoint, definition)

		// FIXME: this leaks implementation details about the docker layer to
		// the lambda host
		args := docker.RunContainerArgs{
			ContainerName: containerName,
			ImageName:     imageName,
			Handler:       definition.Handler,
			SourcePath:    path.Join(opts.RootDir, definition.LogicalID),
			Port:          containerPort,
		}

		host := lambdahost.New(cli, args)
		go host.Run(dockerCtx, done, &wg)
		defer host.RemoveContainer(dockerCtx)
		lambdaHosts = append(lambdaHosts, host)

		srv.AddRoute(string(endpoint.Method), endpoint.URLPath, args.Port)
		containerIdx++
		containerPort++

		endpointStrings = append(endpointStrings,
			fmt.Sprintf(" - %s http://%s:%d%s\n", string(endpoint.Method), opts.Host, opts.Port, endpoint.URLPath))

		watchPath := path.Join(opts.RootDir, definition.LogicalID)
		log.Debug().Str("path", watchPath).Msg("adding path to watch list")
		if err := watcher.Add(watchPath); err != nil {
			log.Warn().Err(err).Str("path", watchPath).Msg("could not watch directory")
		}

	}

	srv.Run()

	// print information for the user
	wg.Wait()
	fmt.Fprintf(os.Stderr, "Server listening\n")
	fmt.Fprintf(os.Stderr, "Available endpoints:\n")
	for _, s := range endpointStrings {
		fmt.Fprintf(os.Stderr, s)
	}

	// helper function to print info for the user
	printShuttingDown := func() {
		fmt.Fprintf(os.Stderr, "Shutting down the server\n")
	}

	for {
		select {
		case <-ctx.Done():
			log.Debug().Msg("got context timeout")
			srv.Shutdown()
			for _, host := range lambdaHosts {
				host.Shutdown()
			}
			printShuttingDown()
			return nil
		case <-c:
			log.Debug().Msg("got ctrl-c")
			srv.Shutdown()
			for _, host := range lambdaHosts {
				host.Shutdown()
			}
			printShuttingDown()
			return nil
		case event, ok := <-watcher.Events:
			log.Debug().Interface("event", event).Msg("got event")
			if !ok {
				continue
			}

			if event.Op&fsnotify.Write == fsnotify.Write {
				log.Debug().Msg("modified file")
				for _, host := range lambdaHosts {
					host.Restart()
				}
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				continue
			}

			log.Warn().Err(err).Msg("error watching code")
		case <-done:
			return nil
		}
	}
}

func main() {
	// so we can generate random names across multiple running copies of the
	// binary
	rand.Seed(time.Now().UnixNano())

	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{
		Out: os.Stderr,
	})

	var opts Opts
	_, err := flags.Parse(&opts)
	if err != nil {
		// special error handling - the flags package prints the help for us
		os.Exit(1)
	}

	switch len(opts.Verbose) {
	case 0:
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	case 1:
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	default:
		zerolog.SetGlobalLevel(zerolog.TraceLevel)
	}

	log.Debug().Interface("opts", opts).Msg("parsed command line options")

	ctx := context.TODO()
	if err := run(ctx, opts); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
