COVERAGE_THRESHOLD ?= 90

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

.PHONY: coverage
coverage:
	go test -covermode=atomic -coverprofile=coverage.out ./...
	go tool cover -func=$(CURDIR)/coverage.out
	@go tool cover -func=$(CURDIR)/coverage.out | awk -v threshold="$(COVERAGE_THRESHOLD)" '/^total:/ { coverage=$$3; sub(/%/, "", coverage); if (coverage + 0 < threshold + 0) { printf("coverage %.1f%% is below %.1f%%\n", coverage, threshold); exit 1 } printf("coverage %.1f%% meets %.1f%% threshold\n", coverage, threshold) }'

.PHONY: vuln
vuln:
	go tool govulncheck ./...

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: build
build:
	go build -o ./bin/git-kura ./cmd/kura

.PHONY: check
check: fmt-check vet coverage vuln
