package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

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
	port   int

	routes []routeDefinition
}

func New(port int) *Server {
	return &Server{
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

// type cancelFunc func()

// Run runs the web server in the background
//
// Call the cancelFunc to stop the web server
func (s *Server) Run() error {
	if s.server != nil {
		panic("server already created")
	}

	router := mux.NewRouter()
	for _, route := range s.routes {
		router.HandleFunc(route.path, handleRequest(route.path, route.port)).Methods(route.method)
	}

	s.server = &http.Server{
		Addr:    fmt.Sprintf("localhost:%d", s.port),
		Handler: router,
	}

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("server failed")
		}
	}()

	return nil
}

func (s *Server) Restart() {
	s.Shutdown()
	s.Run()
}

func (s *Server) Shutdown() {
	// shortcut if the server hasn't been run yet
	if s.server == nil {
		return
	}

	if err := s.server.Shutdown(context.TODO()); err != nil {
		log.Fatal().Err(err).Msg("server shutdown failed")
	}

	s.server = nil
}

func handleRequest(endpoint string, port int) http.HandlerFunc {
	type rawResponse struct {
		StatusCode int    `json:"statusCode"`
		Body       string `json:"body"`
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

		logger.Debug().Interface("decoded_response", raw).Msg("response ok")
		w.WriteHeader(raw.StatusCode)
		w.Write([]byte(raw.Body))
	}
}
