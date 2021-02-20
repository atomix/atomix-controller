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
	go build -o build/bin/atomix-controller ./cmd/controller
	go build -o build/bin/atomix-coordinator ./cmd/coordinator
	go build -o build/bin/atomix-controller-init ./cmd/init

controller-image:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o build/docker/controller/bin/atomix-controller ./cmd/controller
	docker build ./build/docker/controller -f build/docker/controller/Dockerfile -t atomix/kubernetes-controller:${CONTROLLER_VERSION}

coordinator-image:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o build/docker/coordinator/bin/atomix-coordinator ./cmd/coordinator
	docker build ./build/docker/coordinator -f build/docker/coordinator/Dockerfile -t atomix/kubernetes-coordinator:${CONTROLLER_VERSION}

init-image:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o build/docker/init/bin/atomix-controller-init ./cmd/init
	docker build ./build/docker/init -f build/docker/init/Dockerfile -t atomix/kubernetes-controller-init:${CONTROLLER_VERSION}

images: controller-image coordinator-image init-image

kind: images
	@if [ "`kind get clusters`" = '' ]; then echo "no kind cluster found" && exit 1; fi
	kind load docker-image atomix/kubernetes-controller:${CONTROLLER_VERSION}
	kind load docker-image atomix/kubernetes-coordinator:${CONTROLLER_VERSION}
	kind load docker-image atomix/kubernetes-controller-init:${CONTROLLER_VERSION}

push: # @HELP push kubernetes-controller Docker image
	docker push atomix/kubernetes-controller:${CONTROLLER_VERSION}
	docker push atomix/kubernetes-coordinator:${CONTROLLER_VERSION}
	docker push atomix/kubernetes-controller-init:${CONTROLLER_VERSION}

test: # @HELP run the unit tests and source code validation
test: deps license_check linters
	go test github.com/atomix/kubernetes-controller/cmd/...
	go test github.com/atomix/kubernetes-controller/pkg/...

deps: # @HELP ensure that the required dependencies are in place
	go build -v ./...

linters: # @HELP examines Go source code and reports coding problems
	golangci-lint run

license_check: # @HELP examine and ensure license headers exist
	./build/licensing/boilerplate.py -v
