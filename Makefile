.PHONY: all

# Go parameters
GOCMD=go
GOBUILD=CGO_ENABLED=1 $(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test -count=1
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

BINDIR=bin

# Platform-specific server process check
ifeq ($(OS),Windows_NT)
    SERVER_BINARY_NAME=hyperspot-server.exe
    SERVER_BINARY_NAME_INSTRUMENTED=hyperspot-server-instrumented.exe
    CLIENT_BINARY_NAME=hyperspot-client.exe
    CLIENT_BINARY_NAME_INSTRUMENTED=hyperspot-client-instrumented.exe
    ifneq (,$(filter MSYS MINGW32 MINGW64 UCRT64 CLANG64,$(MSYSTEM)))
        CHECK_SERVER_RUNNING=tasklist -FI "IMAGENAME eq $(SERVER_BINARY_NAME)" | findstr hyperspot-server > /dev/null
        CHECK_INSTRUMENTED_SERVER_RUNNING=tasklist -FI "IMAGENAME eq $(SERVER_BINARY_NAME_INSTRUMENTED)" | findstr hyperspot-server > /dev/null
	KILL_INSTRUMENTED_SERVER_RUNNING=pkill -f $(SERVER_BINARY_NAME_INSTRUMENTED)
    else
        CHECK_SERVER_RUNNING=tasklist /FI "IMAGENAME eq $(SERVER_BINARY_NAME)" | findstr hyperspot-server > NUL
        CHECK_INSTRUMENTED_SERVER_RUNNING=tasklist /FI "IMAGENAME eq $(SERVER_BINARY_NAME_INSTRUMENTED)" | findstr $(SERVER_BINARY_NAME_INSTRUMENTED) > NUL
	KILL_INSTRUMENTED_SERVER_RUNNING=taskkill /IM $(SERVER_BINARY_NAME_INSTRUMENTED) /F
    endif
else
    SERVER_BINARY_NAME=hyperspot-server
    SERVER_BINARY_NAME_INSTRUMENTED=hyperspot-server-instrumented
    CLIENT_BINARY_NAME=hyperspot-client
    CLIENT_BINARY_NAME_INSTRUMENTED=hyperspot-client-instrumented
    UNAME_S := $(shell uname -s)
    ifeq ($(UNAME_S),Darwin)
        CHECK_SERVER_RUNNING=pgrep -f $(SERVER_BINARY_NAME) > /dev/null
        CHECK_INSTRUMENTED_SERVER_RUNNING=pgrep -f $(SERVER_BINARY_NAME_INSTRUMENTED) > /dev/null
		KILL_INSTRUMENTED_SERVER_RUNNING=pkill -f $(SERVER_BINARY_NAME_INSTRUMENTED)
    else
        CHECK_SERVER_RUNNING=pgrep -f "[/]$(SERVER_BINARY_NAME)" > /dev/null
        CHECK_INSTRUMENTED_SERVER_RUNNING=pgrep -f "[/]$(SERVER_BINARY_NAME_INSTRUMENTED)" > /dev/null
		KILL_INSTRUMENTED_SERVER_RUNNING=pkill -f "[/]$(SERVER_BINARY_NAME_INSTRUMENTED)"
    endif
endif

SERVER_BINARY_PATH=$(PWD)/$(BINDIR)/$(SERVER_BINARY_NAME)
SERVER_BINARY_PATH_INSTRUMENTED=$(PWD)/$(BINDIR)/$(SERVER_BINARY_NAME_INSTRUMENTED)

CLIENT_BINARY_PATH=$(PWD)/$(BINDIR)/$(CLIENT_BINARY_NAME)
CLIENT_BINARY_PATH_INSTRUMENTED=$(PWD)/$(BINDIR)/$(CLIENT_BINARY_NAME_INSTRUMENTED)

COVERAGE_ROOT=$(PWD)/coverage/
COVERAGE_DIR=$(PWD)/coverage/last/
COVERAGE_PREV_DIR=$(COVERAGE_ROOT)$(shell date +%Y-%m-%d_%H-%M-%S)/
COVERAGE_SERVER_DATA_DIR=$(COVERAGE_DIR)data/server/
COVERAGE_CLIENT_DATA_DIR=$(COVERAGE_DIR)data/client/
COVERAGE_SERVER_UNIT_TESTS_PFX=$(COVERAGE_DIR)server-unit-tests-coverage
COVERAGE_SERVER_API_TESTS_PFX=$(COVERAGE_DIR)server-api-tests-coverage
COVERAGE_SERVER_APISPEC_TESTS_PFX=$(COVERAGE_DIR)server-apispec-tests-coverage
COVERAGE_CLIENT_API_TESTS_PFX=$(COVERAGE_DIR)client-api-tests-coverage
COVERAGE_SERVER_STRESS_TESTS_PFX=$(COVERAGE_DIR)server-stress-tests-coverage
COVERAGE_CLIENT_STRESS_TESTS_PFX=$(COVERAGE_DIR)client-stress-tests-coverage
COVERAGE_TOTAL_TESTS_PFX=$(COVERAGE_DIR)total-coverage

GOCOVMERGE=gocovmerge
GOLINTER=golangci-lint

# Required for weak pointers (for in-memory caches)
MIN_GO_VERSION=1.20

all: clean build unit-tests test-races cross-build check-prereqs lint api-tests docsgen api-spec-tests config-tests stress-tests coverage-report ok

check-running:
	@echo "====================================================================================="
	@echo "Checking if hyperspot server is not running already ..."
	@echo "-------------------------------------------------------------------------------------"
	@if $(CHECK_INSTRUMENTED_SERVER_RUNNING); then \
	    echo "Error: Instrumented server is already running, aborting" ; \
	    exit 1 ; \
	fi
	@if $(CHECK_SERVER_RUNNING); then \
	    echo "Error: can't start instrumented server, because another server is already running" ; \
	    exit 1 ; \
	fi
	@echo "OK, server is not running"
	@echo ""

check-prereqs:
	@echo "====================================================================================="
	@echo "Checking if current environment has all required prereqs for tests execution ..."
	@echo "-------------------------------------------------------------------------------------"
	python3 ./py-tests/check_test_env.py ; \
	status=$$? ; \
	if [ $$status -ne 0 ]; then \
	    echo "Current environment is not fully ready for tests execution." ; \
	    exit $$status ; \
	else \
	    echo "OK, current environment is ready for tests execution." ; \
	fi
	@echo ""

ok:
	@echo "====================================================================================="
	@echo "OK, all tests passed"
	@echo "-------------------------------------------------------------------------------------"

coverage-report:
	@echo "====================================================================================="
	@echo "Generating coverage report..."
	@echo "-------------------------------------------------------------------------------------"
	@$(GOCMD) install github.com/wadey/gocovmerge@latest
	@$(GOCOVMERGE) $(COVERAGE_SERVER_UNIT_TESTS_PFX).out $(COVERAGE_SERVER_API_TESTS_PFX).out $(COVERAGE_CLIENT_API_TESTS_PFX).out $(COVERAGE_SERVER_STRESS_TESTS_PFX).out $(COVERAGE_CLIENT_STRESS_TESTS_PFX).out > $(COVERAGE_TOTAL_TESTS_PFX).out
	@$(GOCMD) tool cover -func=$(COVERAGE_TOTAL_TESTS_PFX).out > $(COVERAGE_TOTAL_TESTS_PFX).txt
	@$(GOCMD) tool cover -html=$(COVERAGE_TOTAL_TESTS_PFX).out -o $(COVERAGE_TOTAL_TESTS_PFX).html
	@echo "Total coverage report generated in $(COVERAGE_TOTAL_TESTS_PFX).html"
	@echo "Total coverage: "
	tail -n 1 $(COVERAGE_TOTAL_TESTS_PFX).txt | sed -E 's/[[:space:]]+/ /g'
	@echo ""

build:
	@echo "====================================================================================="
	@echo "Building..."
	@echo "-------------------------------------------------------------------------------------"
	@mkdir -p $(BINDIR)
	$(GOBUILD) -o $(BINDIR)/$(SERVER_BINARY_NAME) ./cmd/hyperspot-server
	$(GOBUILD) -o $(BINDIR)/$(CLIENT_BINARY_NAME) ./cmd/hyperspot-client
	@echo ""


cross-build:
	@echo "====================================================================================="
	@echo "Checking cross-build for multiple platforms..."
	@echo "-------------------------------------------------------------------------------------"
	@mkdir -p $(BINDIR)/cross-platform
	@echo "Building for Windows x64..."
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GOCMD) build -o $(BINDIR)/cross-platform/hyperspot-server-windows-amd64.exe ./cmd/hyperspot-server
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GOCMD) build -o $(BINDIR)/cross-platform/hyperspot-client-windows-amd64.exe ./cmd/hyperspot-client
	@echo "Building for Windows ARM64..."
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 $(GOCMD) build -o $(BINDIR)/cross-platform/hyperspot-server-windows-arm64.exe ./cmd/hyperspot-server
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 $(GOCMD) build -o $(BINDIR)/cross-platform/hyperspot-client-windows-arm64.exe ./cmd/hyperspot-client
	@echo "Building for macOS x64..."
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GOCMD) build -o $(BINDIR)/cross-platform/hyperspot-server-darwin-amd64 ./cmd/hyperspot-server
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GOCMD) build -o $(BINDIR)/cross-platform/hyperspot-client-darwin-amd64 ./cmd/hyperspot-client
	@echo "Building for macOS ARM64 (Apple Silicon)..."
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GOCMD) build -o $(BINDIR)/cross-platform/hyperspot-server-darwin-arm64 ./cmd/hyperspot-server
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GOCMD) build -o $(BINDIR)/cross-platform/hyperspot-client-darwin-arm64 ./cmd/hyperspot-client
	@echo "Building for Linux x64..."
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOCMD) build -o $(BINDIR)/cross-platform/hyperspot-server-linux-amd64 ./cmd/hyperspot-server
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOCMD) build -o $(BINDIR)/cross-platform/hyperspot-client-linux-amd64 ./cmd/hyperspot-client
	@echo "Building for Linux ARM64..."
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GOCMD) build -o $(BINDIR)/cross-platform/hyperspot-server-linux-arm64 ./cmd/hyperspot-server
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GOCMD) build -o $(BINDIR)/cross-platform/hyperspot-client-linux-arm64 ./cmd/hyperspot-client
	@echo "Cross-platform build completed. Binaries available in $(BINDIR)/cross-platform/"
	@echo ""
	@echo "Built binaries:"
	@ls -lah $(BINDIR)/cross-platform/* | awk 'NR>1 {printf " - %-60s %8s\n", $$9, $$5}'
	@echo ""
	@echo "OK, cross-compilation completed"
	@echo ""

test-races:
	@echo "====================================================================================="
	@echo "Running race condition tests ..."
	@echo "-------------------------------------------------------------------------------------"
	$(GOTEST) -race ./...
	$(GOTEST) ./...
	$(GOTEST) ./...
	$(GOTEST) ./...
	$(GOTEST) ./...
	$(GOTEST) ./...
	@echo ""

mkdir-coverage-dirs:
	@mkdir -p $(BINDIR)
	@mkdir -p $(COVERAGE_PREV_DIR)
	@mkdir -p $(COVERAGE_DIR)
	@mkdir -p $(COVERAGE_SERVER_DATA_DIR)
	@mkdir -p $(COVERAGE_CLIENT_DATA_DIR)

api-tests: check-running mkdir-coverage-dirs
	@echo "====================================================================================="
	@echo "Running API tests with coverage..."
	@echo "-------------------------------------------------------------------------------------"
	$(GOBUILD) -cover -o $(BINDIR)/$(SERVER_BINARY_NAME_INSTRUMENTED) -covermode=count -coverpkg=./... ./cmd/hyperspot-server
	$(GOBUILD) -cover -o $(BINDIR)/$(CLIENT_BINARY_NAME_INSTRUMENTED) -covermode=count -coverpkg=./... ./cmd/hyperspot-client
	@echo "Starting instrumented server..."
	GOCOVERDIR=$(COVERAGE_SERVER_DATA_DIR) $(SERVER_BINARY_PATH_INSTRUMENTED) -vv -mock 2>&1 > $(COVERAGE_DIR)/api-tests-server.log &
	@sleep 1
	@if $(CHECK_INSTRUMENTED_SERVER_RUNNING); then \
	    echo "OK, server has been started successfully" ; \
	else \
	    echo "Error: Instrumented server failed to start. Aborting." ; \
	    exit 1 ; \
	fi
	python3 ./py-tests/api_tests.py --coverage-dir $(COVERAGE_CLIENT_DATA_DIR) --client $(CLIENT_BINARY_PATH_INSTRUMENTED) ; \
	status=$$? ; \
	echo "Stopping server gracefully..." ; \
	$(KILL_INSTRUMENTED_SERVER_RUNNING) ; \
	echo "Server gracefully terminated." ; \
	if [ $$status -ne 0 ]; then \
	    echo "API tests failed with exit code $$status. Aborting." ; \
		echo "See server logs in $(COVERAGE_DIR)/api-tests-server.log" ; \
	    exit $$status ; \
	fi
	@echo "Collecting API tests coverage information..."
	$(GOCMD) tool covdata textfmt -i=$(COVERAGE_SERVER_DATA_DIR) -o $(COVERAGE_SERVER_API_TESTS_PFX).out
	$(GOCMD) tool covdata textfmt -i=$(COVERAGE_CLIENT_DATA_DIR) -o $(COVERAGE_CLIENT_API_TESTS_PFX).out
	$(GOCMD) tool cover -func=$(COVERAGE_SERVER_API_TESTS_PFX).out > $(COVERAGE_SERVER_API_TESTS_PFX).txt
	$(GOCMD) tool cover -html=$(COVERAGE_SERVER_API_TESTS_PFX).out -o $(COVERAGE_SERVER_API_TESTS_PFX).html
	@echo "API tests coverage report generated in $(COVERAGE_SERVER_API_TESTS_PFX).html"
	$(GOCMD) tool cover -func=$(COVERAGE_CLIENT_API_TESTS_PFX).out > $(COVERAGE_CLIENT_API_TESTS_PFX).txt
	$(GOCMD) tool cover -html=$(COVERAGE_CLIENT_API_TESTS_PFX).out -o $(COVERAGE_CLIENT_API_TESTS_PFX).html
	cp -a $(COVERAGE_DIR)/* $(COVERAGE_PREV_DIR)/
	@echo "API tests coverage report generated in $(COVERAGE_CLIENT_API_TESTS_PFX).html"
	@echo ""

api-spec-tests:
	@echo "====================================================================================="
	@echo "Running API Specification tests ..."
	@echo "-------------------------------------------------------------------------------------"
	python3 ./py-tests/apispec_tests.py ; \
	status=$$? ; \
	if [ $$status -ne 0 ]; then \
	    echo "API Specification tests failed with exit code $$status. Aborting." ; \
		echo "See server logs in $(COVERAGE_DIR)/api-tests-server.log" ; \
	    exit $$status ; \
	fi
	@echo ""

config-tests:
	@echo "====================================================================================="
	@echo "Running config tests..."
	@echo "-------------------------------------------------------------------------------------"
	$(GOBUILD) -o $(BINDIR)/$(SERVER_BINARY_NAME) ./cmd/hyperspot-server
	$(GOBUILD) -o $(BINDIR)/$(CLIENT_BINARY_NAME) ./cmd/hyperspot-client
	@python3 ./py-tests/config_tests.py
	@echo ""

stress-tests: check-running mkdir-coverage-dirs
	@echo
	@echo "====================================================================================="
	@echo "Running stress tests with coverage..."
	@echo "-------------------------------------------------------------------------------------"
	$(GOBUILD) -cover -o $(BINDIR)/$(SERVER_BINARY_NAME_INSTRUMENTED) -covermode=count -coverpkg=./... ./cmd/hyperspot-server
	$(GOBUILD) -cover -o $(BINDIR)/$(CLIENT_BINARY_NAME_INSTRUMENTED) -covermode=count -coverpkg=./... ./cmd/hyperspot-client
	@echo "Starting instrumented server..."
	GOCOVERDIR=$(COVERAGE_SERVER_DATA_DIR) $(SERVER_BINARY_PATH_INSTRUMENTED) -vv -mock 2>&1 > $(COVERAGE_DIR)/stress-tests-server.log &
	@sleep 1
	@if $(CHECK_INSTRUMENTED_SERVER_RUNNING); then \
	    echo "OK, server has been started successfully" ; \
	else \
	    echo "Error: Instrumented server failed to start. Aborting." ; \
	    exit 1 ; \
	fi
	python3 ./py-tests/stress_tests.py --coverage-dir $(COVERAGE_CLIENT_DATA_DIR) --client $(CLIENT_BINARY_PATH_INSTRUMENTED) ; \
	status=$$? ; \
	echo "Stopping server gracefully..." ; \
	$(KILL_INSTRUMENTED_SERVER_RUNNING) ; \
	echo "Server gracefully terminated." ; \
	if [ $$status -ne 0 ]; then \
	    echo "Stress tests failed with exit code $$status. Aborting." ; \
		echo "See server logs in $(COVERAGE_DIR)/stress-tests-server.log" ; \
	    exit $$status ; \
	fi
	@echo "Collecting API tests coverage information..."
	$(GOCMD) tool covdata textfmt -i=$(COVERAGE_SERVER_DATA_DIR) -o $(COVERAGE_SERVER_STRESS_TESTS_PFX).out
	$(GOCMD) tool covdata textfmt -i=$(COVERAGE_CLIENT_DATA_DIR) -o $(COVERAGE_CLIENT_STRESS_TESTS_PFX).out
	$(GOCMD) tool cover -func=$(COVERAGE_SERVER_STRESS_TESTS_PFX).out > $(COVERAGE_SERVER_STRESS_TESTS_PFX).txt
	$(GOCMD) tool cover -html=$(COVERAGE_SERVER_STRESS_TESTS_PFX).out -o $(COVERAGE_SERVER_STRESS_TESTS_PFX).html
	@echo "API tests coverage report generated in $(COVERAGE_SERVER_STRESS_TESTS_PFX).html"
	$(GOCMD) tool cover -func=$(COVERAGE_CLIENT_STRESS_TESTS_PFX).out > $(COVERAGE_CLIENT_STRESS_TESTS_PFX).txt
	$(GOCMD) tool cover -html=$(COVERAGE_CLIENT_STRESS_TESTS_PFX).out -o $(COVERAGE_CLIENT_STRESS_TESTS_PFX).html
	cp -a $(COVERAGE_DIR)/* $(COVERAGE_PREV_DIR)/
	@echo "API tests coverage report generated in $(COVERAGE_CLIENT_STRESS_TESTS_PFX).html"
	@echo ""

clean:
	@echo
	@echo "====================================================================================="
	@echo "Cleaning..."
	@echo "-------------------------------------------------------------------------------------"
	$(GOCLEAN)
	rm -rf $(BINDIR)/*
	rm -rf $(BINDIR)/cross-platform
		@if [ -z "$(COVERAGE_SERVER_DATA_DIR)" ]; then \
		echo "COVERAGE_SERVER_DATA_DIR is not set"; \
		exit 1; \
	fi
	@if [ -z "$(COVERAGE_SERVER_DATA_DIR)" ]; then \
		echo "COVERAGE_SERVER_DATA_DIR is not set"; \
		exit 1; \
	fi
	@mkdir -p $(COVERAGE_SERVER_DATA_DIR)
	@mkdir -p $(COVERAGE_CLIENT_DATA_DIR)
	rm -f $(SERVER_BINARY_PATH_INSTRUMENTED)
	rm -f $(CLIENT_BINARY_PATH_INSTRUMENTED)
	@echo ""


clean-all: clean
	@echo
	@echo "====================================================================================="
	@echo "Cleaning all..."
	@echo "-------------------------------------------------------------------------------------"
	rm -rf $(COVERAGE_ROOT)*
	rm -rf logs/*
	@echo ""

deps:
	@echo
	@echo "====================================================================================="
	@echo "Downloading dependencies..."
	@echo "-------------------------------------------------------------------------------------"
	$(GOMOD) download
	@echo ""

tidy:
	@echo
	@echo "====================================================================================="
	@echo "Tidying dependencies..."
	@echo "-------------------------------------------------------------------------------------"
	$(GOMOD) tidy
	@echo ""

lint:
	@echo
	@echo "====================================================================================="
	@echo "Running linter, it may take a while..."
	@echo "-------------------------------------------------------------------------------------"
	$(GOLINTER) run
	@echo ""

unit-tests:
	@echo "====================================================================================="
	@echo "Running unit tests with coverage..."
	@echo "-------------------------------------------------------------------------------------"
	@mkdir -p $(COVERAGE_DIR)
	@mkdir -p $(COVERAGE_PREV_DIR)
	$(GOTEST) -covermode=count -coverprofile=$(COVERAGE_SERVER_UNIT_TESTS_PFX).out ./...
	$(GOCMD) tool cover -func=$(COVERAGE_SERVER_UNIT_TESTS_PFX).out > $(COVERAGE_SERVER_UNIT_TESTS_PFX).txt
	$(GOCMD) tool cover -html=$(COVERAGE_SERVER_UNIT_TESTS_PFX).out -o $(COVERAGE_SERVER_UNIT_TESTS_PFX).html
	cp -a $(COVERAGE_DIR)/* $(COVERAGE_PREV_DIR)/
	@echo ""

bench:
	@echo "====================================================================================="
	@echo "Running benchmarks..."
	@echo "-------------------------------------------------------------------------------------"
	$(GOTEST) -bench=. ./...
	@echo ""

docsgen:
	@echo "====================================================================================="
	@echo "Generating API documentation..."
	@echo "-------------------------------------------------------------------------------------"
	@mkdir -p $(BINDIR)
	$(GOBUILD) -o $(BINDIR)/$(SERVER_BINARY_NAME) ./cmd/hyperspot-server
	@echo "Starting server to generate OpenAPI specs..."
	$(BINDIR)/$(SERVER_BINARY_NAME) -p 8086 & \
	SERVER_PID=$$! ; \
	sleep 5 ; \
	echo "Downloading OpenAPI specs..." ; \
	mkdir -p docs/api ; \
	curl -k http://127.0.0.1:8086/openapi.json -o docs/api/api.json || { echo "Error downloading OpenAPI JSON"; kill $$SERVER_PID; exit 1; } ; \
	curl -k http://127.0.0.1:8086/openapi.yaml -o docs/api/api.yaml || { echo "Error downloading OpenAPI YAML"; kill $$SERVER_PID; exit 1; } ; \
	echo "Generating Markdown documentation..." ; \
	widdershins docs/api/api.yaml -o docs/api/api.md || { echo "Error generating Markdown documentation"; kill $$SERVER_PID; exit 1; } ; \
	echo "Generating HTML documentation..." ; \
	shins docs/api/api.md -o docs/api/api.html --inline --logo docs/img/hyperspot-logo.png || { echo "Error generating HTML documentation"; kill $$SERVER_PID; exit 1; } ; \
	echo "Stopping server..." ; \
	kill $$SERVER_PID || true ; \
	wait $$SERVER_PID 2>/dev/null || true ; \
	echo "API documentation generated in docs/api/"
	@echo ""

help:
	@awk -F: '/^[a-zA-Z0-9][^$$#\/\t=]*:/ { print $$1 }' Makefile | sort -u
