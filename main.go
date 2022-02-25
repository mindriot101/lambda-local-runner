package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/awslabs/goformation/v5"
	"github.com/awslabs/goformation/v5/cloudformation/serverless"
	"github.com/mindriot101/lambda-local-runner/internal/docker"
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
				}
				out[endpoint] = def
			}

		default:
		}
	}

	return out, nil
}

func buildContainers(ctx context.Context, cli *docker.Client, definitions EndpointMapping) ([]string, error) {
	imageNames := []string{}
	for _, def := range definitions {
		imageName, err := cli.BuildImage(ctx, def.Architecture)
		if err != nil {
			return nil, fmt.Errorf("building image: %w", err)
		}
		imageNames = append(imageNames, imageName)
	}
	return imageNames, nil
}

func runContainers(ctx context.Context, cli *docker.Client, definitions EndpointMapping, imageNames []string) error {
	port := 9001

	i := 0
	for _, def := range definitions {
		imageName := imageNames[i]

		cli.RunContainer(ctx, imageName, def.Handler, def.LogicalID, port)
		port++
		i++
	}
	return nil
}

func main() {
	tplPath := "testproject/template.yaml"
	parsed, err := parseTemplate(tplPath)
	if err != nil {
		panic(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cli, err := docker.New()
	if err != nil {
		panic(err)
	}
	imageNames, err := buildContainers(ctx, cli, parsed)
	if err != nil {
		panic(err)
	}
	if err := runContainers(ctx, cli, parsed, imageNames); err != nil {
		panic(err)
	}
}
