package main

import (
	"strings"
	"testing"
)

func TestContainerName(t *testing.T) {
	endpoint := Endpoint{
		Method:  "get",
		URLPath: "/hello/a/b",
	}
	definition := HandlerDefinition{
		LogicalID: "HelloWorldFunction",
	}

	got := containerName(endpoint, definition)
	prefix := "llr-HelloWorldFunction-get_hello_a_b-"

	if !strings.HasPrefix(got, prefix) {
		t.Fatalf("invalid container name, %s does not begin with %s", got, prefix)
	}

	if len(got) != len(prefix)+6 {
		t.Fatalf("container name should end with 6 random characters")
	}
}

func TestUniqueContainerName(t *testing.T) {
	m := make(map[string]bool)

	endpoint := Endpoint{
		Method:  "get",
		URLPath: "/hello/a/b",
	}
	definition := HandlerDefinition{
		LogicalID: "HelloWorldFunction",
	}

	n := 1024

	for i := 0; i < n; i++ {
		name := containerName(endpoint, definition)
		m[name] = true
	}

	if len(m) != n {
		t.Fatalf("container name not unique enough")
	}
}
