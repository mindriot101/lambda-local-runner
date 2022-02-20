package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/mindriot101/lambda-local-runner/docker"
)

func main() {
	cli, err := docker.New()
	if err != nil {
		panic(err)
	}

	args := docker.RunArgs{
		Image:    "ubuntu:latest",
		MountDir: "/home/simon/dev/lambda-local-runner/testproject",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	go func() {
		for range c {
			cancel()
		}
	}()

	if err := cli.Run(ctx, &args); err != nil {
		panic(err)
	}
}
