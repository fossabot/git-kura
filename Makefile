.PHONY: fmt
fmt:
	gofmt -w $$(find . -name '*.go')

.PHONY: fmt-check
fmt-check:
	@diff=$$(gofmt -d $$(find . -name '*.go')); \
	if [ -n "$$diff" ]; then \
		echo "$$diff"; \
		exit 1; \
	fi

.PHONY: vet
vet:
	go vet ./...

.PHONY: test
test:
	go test ./...

.PHONY: vuln
vuln:
	go tool govulncheck ./...

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: build
build:
	go build -o ./bin/kogoto ./cmd/kogoto

.PHONY: check
check: fmt-check vet test vuln
