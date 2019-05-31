export GOOS=linux
export GOARCH=amd64
export CGO_ENABLED=0

.PHONY: proto build push dev deploy

proto: # @HELP build protobuf services
	docker build -t atomix/atomix-go-build:0.1 build/proto
	docker run -it -v `pwd`:/go/src/github.com/atomix/atomix-k8s-controller atomix/atomix-go-build:0.1 build
build: # @HELP build the controller Docker image
	go build -o build/controller/_output/bin/atomix-k8s-controller ./cmd/controller
	docker build . -f build/controller/Dockerfile -t atomix/atomix-k8s-controller:latest
push: # @HELP push the controller Docker image
	docker push atomix/atomix-k8s-controller:latest
dev: # @HELP run the controller in locally development mode
	WATCH_NAMESPACE=default OPERATOR_NAME=atomix-k8s-controller go run cmd/controller/main.go
deploy: # @HELP deploy the controller to a Kubernetes partition
	kubectl create -f deploy/role.yaml
	kubectl create -f deploy/service_account.yaml
	kubectl create -f deploy/role_binding.yaml
	kubectl create -f deploy/controller.yaml
all: build