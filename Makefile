#!/usr/bin/make -f

REPO ?= $(shell go list -m)
VERSION ?= $(shell git describe --tags --dirty --always)
BUILD ?= $(shell date -u --iso=seconds)
DEBUG ?= false

HUB_IMAGE ?= nspccdev/neofs-http-gw
HUB_TAG ?= "$(shell echo ${VERSION} | sed 's/^v//')"

# List of binaries to build. For now just one.
BINDIR = bin
DIRS = $(BINDIR)
BINS = "$(BINDIR)/neofs-http-gw"

.PHONY: help all dep clean fmts fmt imports test lint docker/lint

# Make all binaries
all: $(BINS)

$(BINS): $(DIRS) dep
	@echo "⇒ Build $@"
	CGO_ENABLED=0 \
	GO111MODULE=on \
	go build -v -trimpath \
	-ldflags "-X main.Version=$(VERSION) \
	-X main.Build=$(BUILD) \
	-X main.Debug=$(DEBUG)" \
	-o $@ ./

$(DIRS):
	@echo "⇒ Ensure dir: $@"
	@mkdir -p $@

# Pull go dependencies
dep:
	@printf "⇒ Download requirements: "
	@CGO_ENABLED=0 \
	GO111MODULE=on \
	go mod download && echo OK
	@printf "⇒ Tidy requirements: "
	@CGO_ENABLED=0 \
	GO111MODULE=on \
	go mod tidy -v && echo OK

# Run all code formatters
fmts: fmt imports

# Reformat code
fmt:
	@echo "⇒ Processing gofmt check"
	@GO111MODULE=on gofmt -s -w ./

# Reformat imports
imports:
	@echo "⇒ Processing goimports check"
	@GO111MODULE=on goimports -w ./

# Build clean Docker image
image:
	@echo "⇒ Build NeoFS HTTP Gateway docker image "
	@docker build \
		--build-arg REPO=$(REPO) \
		--build-arg VERSION=$(VERSION) \
		--rm \
		-f Dockerfile \
		-t $(HUB_IMAGE):$(HUB_TAG) .

# Build dirty Docker image
dirty-image:
	@echo "⇒ Build NeoFS HTTP Gateway dirty docker image "
	@docker build \
		--build-arg REPO=$(REPO) \
		--build-arg VERSION=$(VERSION) \
		--rm \
		-f Dockerfile.dirty \
		-t $(HUB_IMAGE)-dirty:$(HUB_TAG) .

# Run linters
lint:
	@golangci-lint --timeout=5m run

# Run linters in Docker
docker/lint:
	docker run --rm -it \
	-v `pwd`:/src \
	-u `stat -c "%u:%g" .` \
	--env HOME=/src \
	golangci/golangci-lint:v1.39 bash -c 'cd /src/ && make lint'

# Print version
version:
	@echo $(VERSION)

# Show this help prompt
help:
	@echo '  Usage:'
	@echo ''
	@echo '    make <target>'
	@echo ''
	@echo '  Targets:'
	@echo ''
	@awk '/^#/{ comment = substr($$0,3) } comment && /^[a-zA-Z][a-zA-Z0-9_-]+ ?:/{ print "   ", $$1, comment }' $(MAKEFILE_LIST) | column -t -s ':' | grep -v 'IGNORE' | sort -u

# Clean up
clean:
	rm -rf vendor
	rm -rf $(BINDIR)
