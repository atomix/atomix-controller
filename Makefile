export CGO_ENABLED=0
export GO111MODULE=on

.PHONY: build

VERSION := ${ATOMIX_CONTROLLER_VERSION}

all: images

build: # @HELP build the source code
build:
	go build -o build/controller/_output/bin/kubernetes-controller ./cmd/controller

images: # @HELP build kubernetes-controller Docker image
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o build/controller/_output/bin/kubernetes-controller ./cmd/controller
	docker build . -f build/controller/Dockerfile -t atomix/kubernetes-controller:${VERSION}

push: # @HELP push kubernetes-controller Docker image
	docker push atomix/kubernetes-controller:${VERSION}

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
