.PHONY: build

ATOMIX_K8S_CONTROLLER_VERSION := latest

all: image

build: # @HELP build the source code
build:
	go build -o build/controller/_output/bin/atomix-k8s-controller ./cmd/controller

image: # @HELP build atomix-go-raft Docker image
image:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o build/controller/_output/bin/atomix-k8s-controller ./cmd/controller
	docker build . -f build/controller/Dockerfile -t atomix/atomix-k8s-controller:${ATOMIX_K8S_CONTROLLER_VERSION}

proto: # @HELP build Protobuf/gRPC generated types
proto:
	docker run -it -v `pwd`:/go/src/github.com/atomix/atomix-go-raft \
		-w /go/src/github.com/atomix/atomix-go-raft \
		--entrypoint build/bin/compile_protos.sh \
		onosproject/protoc-go:stable

test: # @HELP run the unit tests and source code validation
test: build deps license_check linters
	go test github.com/atomix/atomix-k8s-controller/cmd/...
	go test github.com/atomix/atomix-k8s-controller/pkg/...

coverage: # @HELP generate unit test coverage data
coverage: build deps linters license_check
	./build/bin/coveralls-coverage

deps: # @HELP ensure that the required dependencies are in place
	go build -v ./...
	bash -c "diff -u <(echo -n) <(git diff go.mod)"
	bash -c "diff -u <(echo -n) <(git diff go.sum)"

linters: # @HELP examines Go source code and reports coding problems
	golangci-lint run

license_check: # @HELP examine and ensure license headers exist
	./build/licensing/boilerplate.py -v
