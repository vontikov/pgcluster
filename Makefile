PROJECTNAME = $(shell basename "$(PWD)")

include project.properties

ifndef VERBOSE
.SILENT:
endif

BASE_DIR   = $(shell pwd)
BIN_DIR    = $(BASE_DIR)/.bin
MOCK_DIR   = $(BASE_DIR)/mocks
TMP_DIR    = $(BASE_DIR)/.tmp
TMPL_DIR   = $(BASE_DIR)/assets/templates

GO_PKGS    = $(shell go list ./...)
GO_FILES   = $(shell find . -type f -name '*.go')
GO_LDFLAGS = "-s -w \
  -X github.com/vontikov/pgcluster/internal/app.Namespace=$(NS) \
  -X github.com/vontikov/pgcluster/internal/app.App=$(APP) \
  -X github.com/vontikov/pgcluster/internal/app.Version=$(VERSION)"

MOCKFILES  = \
  internal/concurrent/concurrent.go \
  internal/pg/pg.go \
  internal/storage/storage.go

MOCK_VENDOR_PREFIX = github.com/vontikov/pgcluster/vendor/

ifeq (test,$(firstword $(MAKECMDGOALS)))
  ifneq '$(ALL)' 'true'
    TEST_MODE = -short
  endif
endif

TEST_OPTS  = $(TEST_MODE) -timeout 300s -cover -coverprofile=$(COVERAGE_FILE) -failfast -v
TEST_PKGS  = \
  ./internal/gateway \
  ./internal/pg \
  ./internal/sentinel

GODOG_DEFAULT_CONFIG = stoa.yaml

ifeq (check,$(firstword $(MAKECMDGOALS)))
  ifeq ($(CONFIG),)
    CONFIG := $(GODOG_DEFAULT_CONFIG)
  endif
endif

GODOG_OPTS  = -timeout 300s -failfast -v
GODOG_PKGS  = ./integration

BINARY_FILE  = $(BIN_DIR)/$(APP)

BIN_MARKER   = $(TMP_DIR)/bin.marker
GODOG_MARKER = $(TMP_DIR)/godog.marker
MOCKS_MARKER = $(TMP_DIR)/mocks.marker
TEST_MARKER  = $(TMP_DIR)/test.marker
VET_MARKER   = $(TMP_DIR)/vet.marker

.PHONY: all help prepare clean

all: help

## help: Prints help
help: Makefile
	@echo "Choose a command in "$(PROJECTNAME)":"
	@sed -n 's/^##//p' $< | column -t -s ':' |  sed -e 's/^/ /'

prepare:
	@mkdir -p $(TMP_DIR)
	@mkdir -p $(MOCK_DIR)

## clean: Cleans the project
clean:
	@echo "Cleaning the project..."
	@rm -rf $(BIN_DIR) $(TMP_DIR)

## vet: Runs Go vet
vet: $(VET_MARKER)

$(VET_MARKER): $(GO_FILES)
	@echo "Running Go vet..."
	@GOBIN=$(BIN_DIR); go vet $(GO_PKGS)
	@gosec ./...
	@mkdir -p $(@D)
	@touch $@

## test [ALL=true]: Runs tests
test: $(TEST_MARKER)

$(TEST_MARKER): $(GO_FILES)
	@echo "Executing tests..."
	@mkdir -p $(BIN_DIR)
	@GOBIN=$(BIN_DIR); go test $(TEST_OPTS) $(TEST_PKGS)
	@mkdir -p $(@D)
	@touch $@

## check [CONFIG=(stoa.yaml|etcd.yaml)]: Runs integration tests
check: image $(GODOG_MARKER)

$(GODOG_MARKER): $(GO_FILES)
	@echo "Executing godog tests...\n\n"
	@mkdir -p $(BIN_DIR)
	@GOBIN=$(BIN_DIR); go test $(GODOG_OPTS) $(GODOG_PKGS) --config $(CONFIG)
	@mkdir -p $(@D)
	@touch $@

## build: Builds the binaries
build: $(BIN_MARKER)

$(BIN_MARKER): prepare $(GO_FILES)
	@echo "Building the binaries..."
	@chmod +x $(TMPL_DIR)/version.tmpl
	@$(TMPL_DIR)/version.tmpl $(NS) $(APP) $(VERSION) >$(BASE_DIR)/internal/app/version.go
	@mkdir -p $(BIN_DIR)
	@GOBIN=$(BIN_DIR) \
      CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
      go build \
        -tags=$(BUILD_TAGS) \
        -ldflags=$(GO_LDFLAGS) \
        -o $(BINARY_FILE) cmd/*.go
	@mkdir -p $(@D)
	@touch $@

## image: Builds Docker image
image: build
	@docker build \
      --build-arg VERSION=$(VERSION) \
      -t $(DOCKER_NS)/$(APP):$(VERSION) .

## mocks: Generates mocks
mocks: $(MOCKS_MARKER)

$(MOCKS_MARKER): $(MOCKFILES)
	echo "Generating mocks..."
	$(foreach v,$(MOCKFILES),$(call generate-mock,$v))
	@mkdir -p $(@D)
	@touch $@

define generate-mock
  $(eval dest:=`echo "$(MOCK_DIR)/$(1)" | sed -E 's!vendor/|internal/!!'`)
  echo "$1 -> ${dest}"
  mockgen -source $1 -destination $(dest)
  sed -i 's!$(MOCK_VENDOR_PREFIX)!!g' ${dest}
endef

## deps: Installs dependencies
deps:
	@go mod tidy
	@go install github.com/securego/gosec/v2/cmd/gosec@latest
	@go install github.com/cucumber/godog/cmd/godog@v0.11.0
