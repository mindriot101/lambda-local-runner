package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/docker/docker/client"
	"github.com/gorilla/mux"
	"github.com/mindriot101/lambda-local-runner/internal/docker"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{
		Out: os.Stderr,
	})

	dockerClient, err := client.NewEnvClient()

	if err != nil {
		panic(err)
	}
	cli := docker.New(dockerClient)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	imageName, err := cli.BuildImage(ctx)
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

	go func() {
		if err := cli.RunContainer(ctx, args); err != nil {
			panic(err)
		}
	}()

	// host the container via web server
	webPort := 8080
	router := mux.NewRouter()
	router.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		// TODO: proxy to the lambda
		w.WriteHeader(http.StatusOK)
	})

	srv := &http.Server{
		Addr:    fmt.Sprintf("localhost:%d", webPort),
		Handler: router,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("server failed")
		}
	}()

	select {
	case <-c:
		// cancel running docker containers
		cancel()

		// shutdown web server
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Fatal().Err(err).Msg("server shutdown failed")
		}
		log.Debug().Msg("server shutdown properly")
	}
}
