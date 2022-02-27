.PHONY: help
help:
	@echo "Commands: coverage, test, integration-test"

.PHONY: coverage
coverage:
	go test -coverprofile=c.out ./...
	go tool cover -html=c.out

.PHONY: integration-test
integration-test:
	go test main.go main_test.go -integration

.PHONY: test
test:
	go test ./...
