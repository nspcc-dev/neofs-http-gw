VERSION ?= "$(shell git describe --tags 2>/dev/null | sed 's/^v//')"

GRPC_VERSION=$(shell go list -m google.golang.org/grpc | cut -d " " -f 2)

HUB_IMAGE=nspccdev/neofs

B=\033[0;1m
G=\033[0;92m
R=\033[0m

# Show current version
version:
	@echo "Current version: $(VERSION)-$(GRPC_VERSION)"

# Make sure that all files added to commit
deps:
	@printf "${B}${G}⇒ Ensure vendor${R}: "
	@go mod tidy -v && echo OK || (echo fail && exit 2)
	@printf "${B}${G}⇒ Download requirements${R}: "
	@go mod download && echo OK || (echo fail && exit 2)
	@printf "${B}${G}⇒ Store vendor localy${R}: "
	@go mod vendor && echo OK || (echo fail && exit 2)

image: VERSION?=
image: deps
	@echo "${B}${G}⇒ Build GW docker-image with $(GRPC_VERSION) ${R}"
	@docker build \
		--build-arg VERSION=$(VERSION) \
		 -f Dockerfile \
		 -t $(HUB_IMAGE)-http-gate:$(VERSION) .

.PHONY: dev

# v1.24.0 v1.25.1 v1.26.0 v1.27.1
dev: VERSIONS?=$(GRPC_VERSION)
dev:
	@echo "=> Build multiple images for $(VERSIONS)"; \
	for v in $(VERSIONS); do \
  		curdir=$$(pwd); \
  		echo "=> Checkout gRPC to $${v}"; \
  		cd ../grpc-go; \
  		git checkout $${v} &> /dev/null || (echo  "Release $${v} not found" && exit 2); \
  		cd ../neofs-api; \
  		git checkout go.{sum,mod}; \
  		go get google.golang.org/grpc@$${v}; \
  		cd $${curdir}; \
  		git checkout go.{sum,mod}; \
  		cp  go_dev.mod go.sum; \
  		go get google.golang.org/grpc@$${v}; \
  		make image VERSION=$(VERSION)-$${v}; \
	done