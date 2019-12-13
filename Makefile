VERSION ?= "$(shell git describe --tags 2>/dev/null | sed 's/^v//')"

HUB_IMAGE=nspccdev/neofs

B=\033[0;1m
G=\033[0;92m
R=\033[0m

# Show current version
version:
	@echo $(VERSION)

# Make sure that all files added to commit
deps:
	@printf "${B}${G}⇒ Ensure vendor${R}: "
	@go mod tidy -v && echo OK || (echo fail && exit 2)
	@printf "${B}${G}⇒ Download requirements${R}: "
	@go mod download && echo OK || (echo fail && exit 2)
	@printf "${B}${G}⇒ Store vendor localy${R}: "
	@go mod vendor && echo OK || (echo fail && exit 2)

image: deps
	@echo "${B}${G}⇒ Build GW docker-image ${R}"
	@docker build \
		--build-arg VERSION=$(VERSION) \
		 -f Dockerfile \
		 -t $(HUB_IMAGE)-http-gate:$(VERSION) .
