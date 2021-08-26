.DEFAULT_GOAL := build
BIN_FILE=contrast-agent-injector
DOCKER_IMAGE=ghcr.io/cbuto/contrast-agent-injector
VERSION ?= 0.1.0

.PHONY: build
build:
	@go build -a -o ${BIN_FILE} cmd/injector/main.go

.PHONY: docker-build
docker-build:
	docker build -t ${DOCKER_IMAGE}:${VERSION} .

.PHONY: clean
clean:
	go clean
	rm -f "cover.out"
	rm -f nohup.out

.PHONY: test
test:
	go test ./...

.PHONY: cover
cover:
	go test -coverprofile cover.out ./...
	go tool cover -html=cover.out

.PHONY: cover
lint:
	golangci-lint run

.PHONY: deploy-kind-local
deploy-kind-local:
	time=$$(date +'%Y%m%d-%H%M%S') && \
	docker build -t ghcr.io/cbuto/contrast-agent-injector:$$time . && \
	kind load docker-image ghcr.io/cbuto/contrast-agent-injector:$$time && \
	cd charts/contrast-agent-injector && \
	helm upgrade --set image.tag=$$time --install injector .

.PHONY: deploy-image
deploy-image: docker-build
	docker push ${DOCKER_IMAGE}:${VERSION}
	docker push ${DOCKER_IMAGE}:latest