-include .env
-include help.mk

VERSION ?= "$(shell git describe --tags 2>/dev/null | sed 's/^v//')"

GRPC_VERSION=$(shell go list -m google.golang.org/grpc | cut -d " " -f 2)

HUB_IMAGE=nspccdev/neofs

B=\033[0;1m
G=\033[0;92m
R=\033[0m

.PHONY: version deps image publish

# Show current version
version:
	@echo "Current version: $(VERSION)-$(GRPC_VERSION)"

# Check and ensure dependencies
deps:
	@printf "${B}${G}⇒ Ensure vendor${R}: "
	@go mod tidy -v && echo OK || (echo fail && exit 2)
	@printf "${B}${G}⇒ Download requirements${R}: "
	@go mod download && echo OK || (echo fail && exit 2)
	@printf "${B}${G}⇒ Store vendor localy${R}: "
	@go mod vendor && echo OK || (echo fail && exit 2)

# Build docker image
image: VERSION?=
image: deps
	@echo "${B}${G}⇒ Build GW docker-image with $(GRPC_VERSION) ${R}"
	@docker build \
		--build-arg VERSION=$(VERSION) \
		 -f Dockerfile \
		 -t $(HUB_IMAGE)-http-gate:$(VERSION) .

# Publish docker image
publish:
	@echo "${B}${G}⇒ publish docker image ${R}"
	@docker push $(HUB_IMAGE)-http-gate:$(VERSION)

.PHONY: dev

# Build development docker images
dev: VERSIONS?=$(GRPC_VERSION)
dev:
	@echo "=> Build multiple images for $(VERSIONS)"; \
	git checkout go.{sum,mod}; \
	for v in $(VERSIONS); do \
  		curdir=$$(pwd); \
  		echo "=> Checkout gRPC to $${v}"; \
  		cd ../grpc-go; \
  		git checkout $${v} &> /dev/null || (echo  "Release $${v} not found" && exit 2); \
  		cd ../neofs-api; \
  		git checkout go.{sum,mod}; \
  		go get google.golang.org/grpc@$${v}; \
  		cd $${curdir}; \
  		cp  go_dev.mod go.mod; \
  		go get google.golang.org/grpc@$${v}; \
  		make image VERSION=$(VERSION)-$${v}; \
  		git checkout go.{sum,mod}; \
	done