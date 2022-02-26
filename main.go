package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/docker/docker/client"
	"github.com/fsnotify/fsnotify"
	"github.com/mindriot101/lambda-local-runner/internal/docker"
	"github.com/mindriot101/lambda-local-runner/internal/lambdahost"
	"github.com/mindriot101/lambda-local-runner/internal/server"
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

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

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

	host := lambdahost.New(cli, args)

	done := make(chan struct{})
	go host.Run(done)
	defer host.RemoveContainer()

	srv := server.New(8080)
	srv.AddRoute("GET", "/hello", args.Port)
	srv.Run()

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		panic(err)
	}
	defer watcher.Close()
	watcher.Add("testproject/.aws-sam/build/HelloWorldFunction")

	for {
		select {
		case <-c:
			log.Debug().Msg("got ctrl-c")
			srv.Shutdown()
			host.Shutdown()
		case event, ok := <-watcher.Events:
			log.Debug().Interface("event", event).Msg("got event")
			if !ok {
				continue
			}

			if event.Op&fsnotify.Write == fsnotify.Write {
				log.Debug().Msg("modified file")
				host.Restart()
			}
		case <-done:
			return
		}
	}
}
