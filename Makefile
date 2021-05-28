export CGO_ENABLED=0
export GO111MODULE=on

.PHONY: build

ifdef VERSION
CONTROLLER_VERSION := $(VERSION)
else
CONTROLLER_VERSION := latest
endif

all: images

build: # @HELP build the source code
build:
	go build -o build/bin/atomix-controller ./cmd/atomix-controller
	go build -o build/bin/atomix-broker ./cmd/atomix-broker
	go build -o build/bin/atomix-controller-init-certs ./cmd/atomix-controller-init-certs

controller-image:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -gcflags=-trimpath=${GOPATH} -asmflags=-trimpath=${GOPATH} -o build/docker/atomix-controller/bin/atomix-controller ./cmd/atomix-controller
	docker build ./build/docker/atomix-controller -f build/docker/atomix-controller/Dockerfile -t atomix/atomix-controller:${CONTROLLER_VERSION}

broker-image:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -gcflags=-trimpath=${GOPATH} -asmflags=-trimpath=${GOPATH} -o build/docker/atomix-broker/bin/atomix-broker ./cmd/atomix-broker
	docker build ./build/docker/atomix-broker -f build/docker/atomix-broker/Dockerfile -t atomix/atomix-broker:${CONTROLLER_VERSION}

init-certs-image:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -gcflags=-trimpath=${GOPATH} -asmflags=-trimpath=${GOPATH} -o build/docker/atomix-controller-init-certs/bin/atomix-controller-init-certs ./cmd/atomix-controller-init-certs
	docker build ./build/docker/atomix-controller-init-certs -f build/docker/atomix-controller-init-certs/Dockerfile -t atomix/atomix-controller-init-certs:${CONTROLLER_VERSION}

images: controller-image broker-image init-certs-image

kind: images
	@if [ "`kind get clusters`" = '' ]; then echo "no kind cluster found" && exit 1; fi
	kind load docker-image atomix/atomix-controller:${CONTROLLER_VERSION}
	kind load docker-image atomix/atomix-broker:${CONTROLLER_VERSION}
	kind load docker-image atomix/atomix-controller-init-certs:${CONTROLLER_VERSION}

push: # @HELP push controller Docker image
	docker push atomix/atomix-controller:${CONTROLLER_VERSION}
	docker push atomix/atomix-broker:${CONTROLLER_VERSION}
	docker push atomix/atomix-controller-init-certs:${CONTROLLER_VERSION}

test: # @HELP run the unit tests and source code validation
test: deps license_check linters
	go test github.com/atomix/atomix-controller/cmd/...
	go test github.com/atomix/atomix-controller/pkg/...

deps: # @HELP ensure that the required dependencies are in place
	go build -v ./...

linters: # @HELP examines Go source code and reports coding problems
	golangci-lint run

license_check: # @HELP examine and ensure license headers exist
	./build/licensing/boilerplate.py -v
