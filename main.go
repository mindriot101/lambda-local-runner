package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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

func HandleRequest(port int) http.HandlerFunc {
	type rawResponse struct {
		StatusCode int    `json:"statusCode"`
		Body       string `json:"body"`
	}

	url := fmt.Sprintf("http://localhost:%d/2015-03-31/functions/function/invocations", port)
	return func(w http.ResponseWriter, r *http.Request) {
		logger := log.With().Str("endpoint", "/hello").Logger()
		logger.Debug().Msg("got request")

		var body bytes.Buffer
		if r.Method == "POST" {
			_, err := io.Copy(&body, r.Body)
			if err != nil {
				logger.Error().Msg("could not copy body from request")
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte("could not copy body from request"))
				return
			}
			defer r.Body.Close()
		} else {
			body = *bytes.NewBuffer([]byte("{}"))
		}

		logger.Debug().Msg("creating request to lambda function")
		req, err := http.NewRequest("POST", url, &body)
		if err != nil {
			logger.Error().Msg("could not create child request")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("could not create child request"))
			return
		}

		client := http.Client{}
		logger.Debug().Msg("sending request to lambda container")
		resp, err := client.Do(req)
		if err != nil {
			logger.Error().Msg("could not send request to lambda container")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("error sending request"))
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			logger.Error().Int("status", resp.StatusCode).Msg("invalid status code from endpoint")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("bad status from lambda"))
			return
		}

		var resBody bytes.Buffer
		logger.Debug().Msg("reading response from lambda container")
		if _, err := io.Copy(&resBody, resp.Body); err != nil {
			logger.Error().Msg("could not copy from response")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("could not copy body from response"))
			return
		}

		var raw rawResponse
		if err := json.Unmarshal(resBody.Bytes(), &raw); err != nil {
			logger.Error().Err(err).Msg("could not parse response from lambda")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("invalid lambda response"))
			return
		}

		logger.Debug().Interface("decoded_response", raw).Msg("response ok")
		w.WriteHeader(raw.StatusCode)
		w.Write([]byte(raw.Body))
	}
}

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
	// TODO: somehow use a roundtripper here
	router.HandleFunc("/hello", HandleRequest(9001))

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
