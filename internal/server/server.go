package server

import (
	"context"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/mindriot101/lambda-local-runner/internal/lambdaenv"
	"github.com/rs/zerolog/log"
)

type server struct {
	router *mux.Router
}

func New() *server {
	router := mux.NewRouter()
	return &server{
		router: router,
	}
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *server) Add(function lambdaenv.SpawnArgs) {
	log.Debug().Interface("function", function).Msg("adding function")
	s.router.HandleFunc(function.Endpoint, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func (s *server) Serve(ctx context.Context) error {
	log.Debug().Msg("running web server")
	srv := &http.Server{
		Addr:    "127.0.0.1:8029",
		Handler: s.router,
	}

	go func() {
		log.Debug().Msg("listening")
		if err := srv.ListenAndServe(); err != nil {
			if err != http.ErrServerClosed {
				log.Debug().Err(err).Msg("error with server")
			}
		}
		log.Debug().Msg("server shut down")
	}()

	<-ctx.Done()

	log.Debug().Msg("shutting server down")
	if err := srv.Shutdown(context.TODO()); err != nil {
		log.Debug().Msg("server failed to shut down")
	}

	return nil
}
