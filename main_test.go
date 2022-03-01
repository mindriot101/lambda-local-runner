package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	cp "github.com/otiai10/copy"
)

// This package contains the integration tests which spin up a real docker
// container with the local project.

var integration = flag.Bool("integration", false, "run integration tests")

// TestMain ensures that the SAM build output directory exists
func TestMain(m *testing.M) {
	flag.Parse()
	if *integration {
		if err := cp.Copy("testdata/integration/v1", "testdata/integration/src"); err != nil {
			fmt.Fprintf(os.Stderr, "failed to set up initial data directory: %v\n", err)
			os.Exit(1)
		}
	}
	os.Exit(m.Run())
}

type FirstResponse struct {
	Message string `json:"message"`
}

func (r *FirstResponse) CheckMatches(t *testing.T, body []byte) {
	var got FirstResponse
	err := json.Unmarshal(body, &got)
	if err != nil {
		t.Fatalf("invalid json response")
	}
	if r.Message != got.Message {
		t.Fatalf("invalid message (response: %s, expected %s)", got.Message, r.Message)
	}
}

type SecondResponse struct {
	Message string `json:"message"`
	Foo     int    `json:"foo"`
}

func (r *SecondResponse) CheckMatches(t *testing.T, body []byte) {
	var got SecondResponse
	err := json.Unmarshal(body, &got)
	if err != nil {
		t.Fatalf("invalid json response")
	}
	if r.Message != got.Message {
		t.Fatalf("invalid message (response: %s, expected %s)", got.Message, r.Message)
	}
	if r.Foo != got.Foo {
		t.Fatalf("invalid foo (response: %d, expected %d)", got.Foo, r.Foo)
	}
}

type expectation interface {
	CheckMatches(t *testing.T, body []byte)
}

func TestIntegration(t *testing.T) {
	if !*integration {
		t.Skip("not running integration tests")
	}

	host := "localhost"
	port := 8080

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	opts := Opts{
		RootDir: "testdata/integration/src",
		Args: Args{
			Template: "testdata/integration/template.yaml",
		},
		Host:    host,
		Port:    port,
		Verbose: []bool{true, true, true},
	}

	firstResponse := FirstResponse{
		Message: "hello world",
	}

	errCh := make(chan error)
	go func() {
		t.Logf("starting web server")
		errCh <- run(ctx, opts)
		t.Logf("shutting down web server")
	}()

	assertHTTPResponse(t, host, port, &firstResponse)

	// trigger reload
	secondResponse := SecondResponse{
		Message: "hello other",
		Foo:     42,
	}

	// write the new file contents
	newContents, err := ioutil.ReadFile("testdata/integration/v2/HelloWorldFunction/app.py")
	if err != nil {
		t.Fatalf("reading fixture for file change: %v", err)
	}
	if err := ioutil.WriteFile("testdata/integration/src/HelloWorldFunction/app.py", newContents, 0644); err != nil {
		t.Fatalf("writing new source: %v", err)
	}

	t.Logf("waiting for reload")
	time.Sleep(3 * time.Second)

	assertHTTPResponse(t, host, port, &secondResponse)

	cancel()

	if err := <-errCh; err != nil {
		t.Fatalf("error from run function: %v", err)
	}
}

func assertHTTPResponse(t *testing.T, host string, port int, expected expectation) {
	t.Helper()

	http5XXCount := 0
	for {
		// try to make an HTTP request
		t.Logf("making HTTP request")
		resp, err := http.Get(fmt.Sprintf("http://%s:%d/hello", host, port))
		if err != nil {
			if _, ok := err.(net.Error); ok {
				if strings.Contains(err.Error(), "connection refused") {
					t.Logf("server not up yet")
					time.Sleep(time.Second)
					continue
				}
			} else {
				t.Fatalf("error making HTTP request: %v", err)
			}
		}
		t.Logf("request completed")
		defer resp.Body.Close()

		// assert on the response
		if resp.StatusCode != http.StatusOK {
			if resp.StatusCode == 500 {
				t.Logf("got 5XX, current count: %d", http5XXCount)
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

		expected.CheckMatches(t, b)

		return
	}
}
