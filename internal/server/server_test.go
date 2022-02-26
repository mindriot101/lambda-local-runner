package server

import "testing"

func TestAddRoutes(t *testing.T) {
	server := New(0)
	server.AddRoute("GET", "/hello", 9002)

	if len(server.routes) != 1 {
		t.Fatalf("did not add route")
	}

	checkRouteDefinition(t, server.routes[0], routeDefinition{
		method: "GET",
		path: "/hello",
		port: 9002,
	})
}

func checkRouteDefinition(t *testing.T, got, expected routeDefinition) {
	if got.method != expected.method {
		t.Fatalf("invalid method, expected %s found %s", expected.method, got.method)
	}

	if got.path != expected.path {
		t.Fatalf("invalid path, expected %s found %s", expected.path, got.path)
	}

	if got.port != expected.port {
		t.Fatalf("invalid port, expected %d found %d", expected.port, got.port)
	}
}
