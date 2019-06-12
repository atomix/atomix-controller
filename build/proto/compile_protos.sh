#!/bin/sh

go install ./vendor/github.com/golang/protobuf/protoc-gen-go

rm -rf build/proto/_output/atomix-api
rm -rf proto
git clone --branch master https://github.com/atomix/atomix-api.git build/proto/_output/atomix-api
mv build/proto/_output/atomix-api/proto proto

proto_imports="./proto:${GOPATH}/src/github.com/google/protobuf/src:${GOPATH}/src"

protoc -I=$proto_imports --go_out=proto proto/atomix/headers/*.proto
protoc -I=$proto_imports --go_out=proto proto/atomix/protocols/raft/*.proto
protoc -I=$proto_imports --go_out=proto proto/atomix/protocols/log/*.proto
protoc -I=$proto_imports --go_out=import_path=atomix/controller,plugins=grpc:proto proto/atomix/controller/*.proto
protoc -I=$proto_imports --go_out=Matomix/partition/partition.proto=github.com/atomix/atomix-k8s-controller/proto/atomix/partition,import_path=atomix/controller,plugins=grpc:proto proto/atomix/controller/*.proto
protoc -I=$proto_imports --go_out=Matomix/headers/headers.proto=github.com/atomix/atomix-k8s-controller/proto/atomix/headers,import_path=atomix/counter,plugins=grpc:proto proto/atomix/counter/*.proto
protoc -I=$proto_imports --go_out=Matomix/headers/headers.proto=github.com/atomix/atomix-k8s-controller/proto/atomix/headers,import_path=atomix/election,plugins=grpc:proto proto/atomix/election/*.proto
protoc -I=$proto_imports --go_out=Matomix/headers/headers.proto=github.com/atomix/atomix-k8s-controller/proto/atomix/headers,import_path=atomix/lock,plugins=grpc:proto proto/atomix/lock/*.proto
protoc -I=$proto_imports --go_out=Matomix/headers/headers.proto=github.com/atomix/atomix-k8s-controller/proto/atomix/headers,import_path=atomix/log,plugins=grpc:proto proto/atomix/log/*.proto
protoc -I=$proto_imports --go_out=Matomix/headers/headers.proto=github.com/atomix/atomix-k8s-controller/proto/atomix/headers,import_path=atomix/map,plugins=grpc:proto proto/atomix/map/*.proto
protoc -I=$proto_imports --go_out=Matomix/headers/headers.proto=github.com/atomix/atomix-k8s-controller/proto/atomix/headers,import_path=atomix/set,plugins=grpc:proto proto/atomix/set/*.proto
protoc -I=$proto_imports --go_out=Matomix/headers/headers.proto=github.com/atomix/atomix-k8s-controller/proto/atomix/headers,import_path=atomix/value,plugins=grpc:proto proto/atomix/value/*.proto