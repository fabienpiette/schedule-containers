BIN_DIR ?= bin
BINARY := schedule-containers
CMD := ./cmd/schedule-containers

ifneq (,$(wildcard ./.env))
    include .env
    export
endif

REGISTRY_DOMAIN ?= ghcr.io
REGISTRY_NAMESPACE ?= docker
REGISTRY_USER ?= docker
REGISTRY_PASSWORD ?= changeme
IMAGE_TAG ?= latest

DOCKER_IMAGE ?= $(BINARY):latest
REGISTRY_IMAGE ?= $(REGISTRY_DOMAIN)/$(REGISTRY_NAMESPACE)/$(BINARY):$(IMAGE_TAG)
REGISTRY_REPO ?= $(REGISTRY_NAMESPACE)/$(BINARY)
PUSH_TAGS ?= latest

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: build run test vet lint clean \
	docker-build docker-up docker-down docker-logs docker-push docker-push-tags \
	docker-release docker-verify docker-pull \
	install

build:
	go build -o $(BINARY) $(CMD)

run: build
	DB_PATH=./schedule-containers.db PRESETS_PATH=./presets.yaml ./$(BINARY) serve

test:
	go test ./internal/... -count=1

vet:
	go vet ./...

lint: vet

clean:
	rm -f $(BINARY)
	rm -rf $(BIN_DIR)

install:
	go mod download
	go mod tidy

docker-build:
	docker build --no-cache -t $(DOCKER_IMAGE) .

docker-up:
	docker compose up -d

docker-down:
	docker compose down

docker-logs:
	docker compose logs -f

docker-tag:
	docker tag $(DOCKER_IMAGE) $(REGISTRY_IMAGE)

docker-push:
	docker push $(REGISTRY_IMAGE)

docker-push-tags:
	@if [ -z "$(PUSH_TAGS)" ]; then \
		echo "PUSH_TAGS is required. Invoke as PUSH_TAGS=\"v1.0.0\" make docker-push-tags"; \
		exit 1; \
	fi; \
	for tag in $(PUSH_TAGS); do \
		target_image=$(REGISTRY_DOMAIN)/$(REGISTRY_NAMESPACE)/$(BINARY):$$tag; \
		echo "Tagging $(DOCKER_IMAGE) as $$target_image"; \
		docker tag $(DOCKER_IMAGE) $$target_image; \
		echo "Pushing $$target_image"; \
		docker push $$target_image; \
	done

docker-verify:
	@if [ -z "$(REGISTRY_PASSWORD)" ]; then \
		echo "REGISTRY_PASSWORD is required. Invoke as REGISTRY_PASSWORD=... make docker-verify"; \
		exit 1; \
	fi
	curl -u $(REGISTRY_USER):$(REGISTRY_PASSWORD) https://$(REGISTRY_DOMAIN)/v2/_catalog
	curl -u $(REGISTRY_USER):$(REGISTRY_PASSWORD) https://$(REGISTRY_DOMAIN)/v2/$(REGISTRY_REPO)/tags/list

docker-pull:
	docker pull $(REGISTRY_IMAGE)

docker-release: docker-build docker-tag docker-push
	@echo "Pushing version tag $(VERSION)..."
	docker tag $(DOCKER_IMAGE) $(REGISTRY_DOMAIN)/$(REGISTRY_NAMESPACE)/$(BINARY):$(VERSION)
	docker push $(REGISTRY_DOMAIN)/$(REGISTRY_NAMESPACE)/$(BINARY):$(VERSION)
