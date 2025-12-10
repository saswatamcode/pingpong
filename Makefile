.PHONY: deps
deps: ## Ensures fresh go.mod and go.sum.
	go mod tidy
	go mod verify

.PHONY: format
format: ## Formats Go code.
format:
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: build
build:
	REVISION=$$(git rev-parse HEAD) && \
	BRANCH=$$(git rev-parse --abbrev-ref HEAD) && \
	BUILDUSER=$$(whoami)@$$HOSTNAME && \
	BUILDDATE=$$(date +%Y%m%d-%H:%M:%S) && \
	go build -a \
		-ldflags="-s -w \
		-X github.com/prometheus/common/version.Revision=$$REVISION \
		-X github.com/prometheus/common/version.Branch=$$BRANCH \
		-X github.com/prometheus/common/version.BuildUser=$$BUILDUSER \
		-X github.com/prometheus/common/version.BuildDate=$$BUILDDATE" \
	-o pingpong .