package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strings"
	"testing"
	"time"
)

// This package contains the integration tests which spin up a real docker
// container with the local project.

var integration = flag.Bool("integration", false, "run integration tests")

// TestMain ensures that the SAM build output directory exists
func TestMain(m *testing.M) {
	flag.Parse()
	if *integration {
		info, err := os.Stat("testproject/.aws-sam/build/HelloWorldFunction")
		if err != nil || !info.IsDir() {
			// sam directory doesn't exist
			cmd := exec.Command("sam", "build")
			cmd.Dir = "testproject"
			cmd.Env = os.Environ()
			cmd.Env = append(cmd.Env, "PIP_REQUIRE_VIRTUALENV=0")
			cmd.Stdout = io.Discard
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "failed to build SAM project: %v\n", err)
				os.Exit(1)
			}
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

func (r *FirstResponse) NewFileContents(old []byte) ([]byte, error) {
	s := string(old)

	var b strings.Builder

	replacing := false
	for _, line := range strings.Split(s, "\n") {
		if strings.Contains(line, "REPLACE MARKER START") {
			replacing = true

			// add replacement
			b.WriteString("# REPLACE MARKER START\n")
			b.WriteString("\"message\": \"hello world\",\n")
		} else if strings.Contains(line, "REPLACE MARKER END") {
			replacing = false
		}

		if !replacing {
			b.WriteString(line + "\n")
		}
	}

	out := b.String()
	return []byte(out), nil
}

func (r *SecondResponse) NewFileContents(old []byte) ([]byte, error) {
	s := string(old)

	var b strings.Builder

	replacing := false
	for _, line := range strings.Split(s, "\n") {
		if strings.Contains(line, "REPLACE MARKER START") {
			replacing = true

			// add replacement
			b.WriteString("# REPLACE MARKER START\n")
			b.WriteString("\"message\": \"hello other\",\n\"foo\": 42,\n")
		} else if strings.Contains(line, "REPLACE MARKER END") {
			replacing = false
		}

		if !replacing {
			b.WriteString(line + "\n")
		}
	}

	out := b.String()
	return []byte(out), nil
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
	NewFileContents(old []byte) ([]byte, error)
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
		RootDir: "testproject/.aws-sam/build",
		Args: Args{
			Template: "testproject/template.yaml",
		},
		Host:    host,
		Port:    port,
		Verbose: []bool{true, true, true},
	}

	firstResponse := FirstResponse{
		Message: "hello world",
	}
	handlerFilename := path.Join(opts.RootDir, "HelloWorldFunction", "app.py")
	ensureHandlerContents(t, handlerFilename, &firstResponse)

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
	ensureHandlerContents(t, handlerFilename, &secondResponse)

	t.Logf("waiting for reload")
	time.Sleep(3 * time.Second)

	assertHTTPResponse(t, host, port, &secondResponse)

	cancel()

	err := <-errCh
	if err != nil {
		t.Fatalf("error from run function: %v", err)
	}
}

// ensureHandlerContents makes sure the handler contents in the accompanying
// test project matches the expectation
func ensureHandlerContents(t *testing.T, filename string, res expectation) {
	b, err := ioutil.ReadFile(filename)
	if err != nil {
		t.Fatalf("could not edit file %s: %v", filename, err)
	}
	newContents, err := res.NewFileContents(b)
	if err != nil {
		t.Fatalf("could not generate new file contents %v", err)
	}
	if err := ioutil.WriteFile(filename, newContents, 0644); err != nil {
		t.Fatalf("could not write file contents to file %s: %v", filename, err)
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
