module github.com/atomix/kubernetes-controller

go 1.13

require (
	github.com/atomix/api v0.3.3
	github.com/atomix/api/go v0.3.3
	github.com/atomix/go-framework v0.5.1
	github.com/ghodss/yaml v1.0.0
	github.com/go-logr/logr v0.1.0
	github.com/go-logr/zapr v0.1.0 // indirect
	github.com/gogo/protobuf v1.3.1
	github.com/golang/protobuf v1.4.2
	github.com/google/btree v1.0.0 // indirect
	github.com/google/gofuzz v1.0.0 // indirect
	github.com/google/uuid v1.1.2
	github.com/gregjones/httpcache v0.0.0-20190611155906-901d90724c79 // indirect
	github.com/hashicorp/golang-lru v0.5.1 // indirect
	github.com/imdario/mergo v0.3.6 // indirect
	github.com/mattbaird/jsonpatch v0.0.0-20171005235357-81af80346b1a // indirect
	github.com/pborman/uuid v1.2.0 // indirect
	github.com/peterbourgon/diskv v2.0.1+incompatible // indirect
	github.com/pkg/errors v0.8.1 // indirect
	github.com/spf13/cobra v0.0.5
	golang.org/x/net v0.0.0-20191004110552-13f9640d40b9
	google.golang.org/grpc v1.33.2
	gopkg.in/inf.v0 v0.9.1 // indirect
	k8s.io/api v0.17.2
	k8s.io/apimachinery v0.17.2
	k8s.io/client-go v0.17.2
	k8s.io/code-generator v0.17.2
	k8s.io/utils v0.0.0-20191114184206-e782cd3c129f
	sigs.k8s.io/controller-runtime v0.5.2
	sigs.k8s.io/testing_frameworks v0.1.1 // indirect
)

replace github.com/atomix/api/go => ../atomix-api/go

replace github.com/atomix/go-framework => ../atomix-go-node
