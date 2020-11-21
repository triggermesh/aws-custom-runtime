PACKAGE            = aws-custom-runtime
PACKAGE_DESC       = Triggermesh AWS Lambda Custom Runtime

TARGETS           ?= linux/amd64

BASE_DIR          ?= $(CURDIR)

OUTPUT_DIR        ?= $(BASE_DIR)/_output

BIN_OUTPUT_DIR    ?= $(OUTPUT_DIR)
TEST_OUTPUT_DIR   ?= $(OUTPUT_DIR)
COVER_OUTPUT_DIR  ?= $(OUTPUT_DIR)
DIST_DIR          ?= $(OUTPUT_DIR)

GO                ?= go
GOFMT             ?= gofmt
GOLINT            ?= golangci-lint run
GOTOOL            ?= go tool
GOTEST            ?= gotestsum --junitfile $(TEST_OUTPUT_DIR)/$(PACKAGE)-unit-tests.xml --format pkgname-and-test-fails --

GOPKGS             = ./pkg/events/...
LDFLAGS            = -extldflags=-static -w -s

HAS_GOTESTSUM     := $(shell command -v gotestsum;)
HAS_GOLANGCI_LINT := $(shell command -v golangci-lint;)

.PHONY: help build install release test coverage lint fmt fmt-test clean

all: build

install-gotestsum:
ifndef HAS_GOTESTSUM
	curl -SL https://github.com/gotestyourself/gotestsum/releases/download/v0.4.2/gotestsum_0.4.2_linux_amd64.tar.gz | tar -C $(shell go env GOPATH)/bin -zxf -
endif

install-golangci-lint:
ifndef HAS_GOLANGCI_LINT
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(shell go env GOPATH)/bin v1.26.0
endif

help: ## Display this help
	@awk 'BEGIN {FS = ":.*?## "; printf "\n$(PACKAGE_DESC)\nUsage:\n  make \033[36m<source>\033[0m\n"} /^[a-zA-Z0-9._-]+:.*?## / {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build the binary
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BIN_OUTPUT_DIR)/$(PACKAGE)

test: install-gotestsum ## Run unit tests
	@mkdir -p $(TEST_OUTPUT_DIR)
	$(GOTEST) -p=1 -race -cover -coverprofile=$(TEST_OUTPUT_DIR)/$(PACKAGE)-c.out $(GOPKGS)

cover: test ## Generate code coverage
	@mkdir -p $(COVER_OUTPUT_DIR)
	$(GOTOOL) cover -html=$(TEST_OUTPUT_DIR)/$(PACKAGE)-c.out -o $(COVER_OUTPUT_DIR)/$(PACKAGE)-coverage.html

lint: install-golangci-lint ## Lint source files
	$(GOLINT) $(GOPKGS)

fmt: ## Format source files
	$(GOFMT) -s -w $(shell $(GO) list -f '{{$$d := .Dir}}{{range .GoFiles}}{{$$d}}/{{.}} {{end}} {{$$d := .Dir}}{{range .TestGoFiles}}{{$$d}}/{{.}} {{end}}' $(GOPKGS))

fmt-test: ## Check source formatting
	@test -z $(shell $(GOFMT) -l $(shell $(GO) list -f '{{$$d := .Dir}}{{range .GoFiles}}{{$$d}}/{{.}} {{end}} {{$$d := .Dir}}{{range .TestGoFiles}}{{$$d}}/{{.}} {{end}}' $(GOPKGS)))

release: ## Build release binaries
	@set -e ; \
	for platform in $(TARGETS); do \
		GOOS=$${platform%/*} ; \
		GOARCH=$${platform#*/} ; \
		RELEASE_BINARY=$(PACKAGE)-$${GOOS}-$${GOARCH} ; \
		[ $${GOOS} = "windows" ] && RELEASE_BINARY=$${RELEASE_BINARY}.exe ; \
		echo "GOOS=$${GOOS} GOARCH=$${GOARCH} $(GO) build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$${RELEASE_BINARY}" . ; \
		GOOS=$${GOOS} GOARCH=$${GOARCH} $(GO) build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$${RELEASE_BINARY} . ; \
	done

clean: ## Clean build artifacts
	@for platform in $(TARGETS); do \
		GOOS=$${platform%/*} ; \
		GOARCH=$${platform#*/} ; \
		RELEASE_BINARY=$(PACKAGE)-$${GOOS}-$${GOARCH} ; \
		[ $${GOOS} = "windows" ] && RELEASE_BINARY=$${RELEASE_BINARY}.exe ; \
		$(RM) -v $(DIST_DIR)/$${RELEASE_BINARY}; \
	done
	@$(RM) -v $(BIN_OUTPUT_DIR)/$(PACKAGE)
	@$(RM) -v $(TEST_OUTPUT_DIR)/$(PACKAGE)-c.out $(TEST_OUTPUT_DIR)/$(PACKAGE)-unit-tests.xml
	@$(RM) -v $(COVER_OUTPUT_DIR)/$(PACKAGE)-coverage.html
