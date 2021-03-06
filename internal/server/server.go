package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
)

type routeDefinition struct {
	method string
	path   string
	port   int
}

type Server struct {
	server *http.Server
	host   string
	port   int

	routes []routeDefinition
}

func New(host string, port int) *Server {
	return &Server{
		host: host,
		port: port,
	}
}

func (s *Server) AddRoute(method string, path string, port int) {
	s.routes = append(s.routes, routeDefinition{
		method: method,
		path:   path,
		port:   port,
	})
}

// Run runs the web server in the background
func (s *Server) Run() error {
	if s.server != nil {
		// NOTE: panic is allowed here, as it indicates a programming error,
		// not a runtime error.
		panic("server already created")
	}

	router := mux.NewRouter()
	for _, route := range s.routes {
		router.HandleFunc(route.path, handleRequest(route.path, route.port)).Methods(route.method)
	}

	s.server = &http.Server{
		Addr:    fmt.Sprintf("%s:%d", s.host, s.port),
		Handler: router,
	}

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("server failed")
		}
	}()

	return nil
}

func (s *Server) Shutdown() {
	// shortcut if the server hasn't been run yet
	if s.server == nil {
		return
	}

	// add timeout to server shutdown in case of hanging
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.server.Shutdown(ctx); err != nil {
		log.Fatal().Err(err).Msg("server shutdown failed")
	}

	s.server = nil
}

func handleRequest(endpoint string, port int) http.HandlerFunc {
	type rawResponse struct {
		StatusCode int               `json:"statusCode"`
		Body       string            `json:"body"`
		Headers    map[string]string `json:"headers"`
	}

	url := fmt.Sprintf("http://localhost:%d/2015-03-31/functions/function/invocations", port)
	return func(w http.ResponseWriter, r *http.Request) {
		logger := log.With().Str("endpoint", endpoint).Logger()
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

		if raw.StatusCode == 0 {
			// Something must have gone wrong with the upstream container.
			// Assume this is user error and return a 400.
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(raw.Body))
			return
		}

		logger.Debug().Interface("decoded_response", raw).Msg("response ok")
		for k, v := range raw.Headers {
			w.Header().Add(k, v)
		}
		w.WriteHeader(raw.StatusCode)
		w.Write([]byte(raw.Body))
	}
}
