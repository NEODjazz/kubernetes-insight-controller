APP := k8s-insight-controller
VERSION ?= $(shell cat VERSION)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
IMAGE ?= ghcr.io/neodjazz/kubernetes-insight-controller

LDFLAGS := -s -w \
	-X k8s-insight-controller/internal/version.Version=$(VERSION) \
	-X k8s-insight-controller/internal/version.Commit=$(COMMIT) \
	-X k8s-insight-controller/internal/version.Date=$(DATE)

.PHONY: test vet fmt fmt-check build clean docker-build

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './vendor/*')

fmt-check:
	test -z "$$(gofmt -l $$(find . -name '*.go' -not -path './vendor/*'))"

build:
	mkdir -p bin
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/$(APP) ./cmd/manager

docker-build:
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg DATE=$(DATE) \
		-t $(IMAGE):$(VERSION) .

clean:
	rm -rf bin dist
