package main

import "testing"

func TestContainerName(t *testing.T) {
	endpoint := Endpoint{
		Method:  "get",
		URLPath: "/hello/a/b",
	}
	definition := HandlerDefinition{
		LogicalID: "HelloWorldFunction",
	}

	containerIdx := 0

	got := containerName(endpoint, definition, containerIdx)
	expected := "llr-HelloWorldFunction-get_hello_a_b-0"

	if got != expected {
		t.Fatalf("invalid container name, got %s expected %s", got, expected)
	}
}
