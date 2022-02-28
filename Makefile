.PHONY: help
help:
	@echo "Commands: build, coverage, test, integration-test"

.PHONY: build
build:
	CGO_ENABLED=0 go build -ldflags '-w -s' -o lambda-local-runner

.PHONY: coverage
coverage:
	go test -coverprofile=c.out ./...
	go tool cover -html=c.out

.PHONY: integration-test
integration-test:
	go test -race main.go main_test.go -integration

.PHONY: test
test:
	go test -race ./...

.PHONY: test-all
test-all:
	$(MAKE) test integration-test
