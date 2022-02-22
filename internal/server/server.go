package server

import (
	"context"

	"github.com/mindriot101/lambda-local-runner/internal/lambdaenv"
	"github.com/rs/zerolog/log"
)

type server struct {
	functions []lambdaenv.SpawnArgs
}

func New() *server {
	return &server{}
}

func (s *server) Add(function lambdaenv.SpawnArgs) {
	log.Debug().Interface("function", function).Msg("adding function")
	s.functions = append(s.functions, function)
}

func (s *server) Serve(ctx context.Context) error {
	log.Debug().Msg("running web server")
	return nil
}
