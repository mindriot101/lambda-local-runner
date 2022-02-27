package main

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

// This package contains the integration tests which spin up a real docker
// container with the local project.

func TestIntegration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	opts := Opts{
		RootDir: "testproject/.aws-sam/build",
		Args: Args{
			Template: "testproject/template.yaml",
		},
		Verbose: []bool{true, true, true},
	}

	errCh := make(chan error)
	go func() {
		errCh <- run(ctx, opts)
	}()

	http5XXCount := 0
	for {
		// try to make an HTTP request
		log.Printf("making HTTP request")
		resp, err := http.Get("http://localhost:8080/hello")
		if err != nil {
			if _, ok := err.(net.Error); ok {
				if strings.Contains(err.Error(), "connection refused") {
					log.Printf("server not up yet")
					time.Sleep(time.Second)
					continue
				}
			} else {
				t.Fatalf("error making HTTP request: %v", err)
			}
		}
		log.Printf("request completed")
		defer resp.Body.Close()

		// assert on the response
		if resp.StatusCode != http.StatusOK {
			if resp.StatusCode == 500 {
				log.Printf("got 5XX, current count: %d", http5XXCount)
				http5XXCount++

				if http5XXCount >= 5 {
					t.Fatalf("too many 5XX failures")
				}

				time.Sleep(time.Second)
				continue
			} else {
				t.Fatalf("bad status code %d", resp.StatusCode)
			}
		}

		b, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("could not read response body: %v", err)
		}

		type responsePayload struct {
			Message string `json:"message"`
		}
		var p responsePayload
		if err := json.Unmarshal(b, &p); err != nil {
			t.Fatalf("invalid JSON in response: %v", err)
		}

		if p.Message != "hello world" {
			t.Fatalf("invalid response body, got %s expected `hello world`", p.Message)
		}

		cancel()
		break
	}
	err := <-errCh
	if err != nil {
		t.Fatalf("error from run function: %v", err)
	}
}