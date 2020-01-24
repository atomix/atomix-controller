# Atomix Kubernetes Controller

[![Build Status](https://travis-ci.org/atomix/k8s-controller.svg?branch=master)](https://travis-ci.org/atomix/k8s-controller)
[![Integration Test Status](https://img.shields.io/travis/atomix/k8s-controller?label=Atomix%20Tests&logo=Atomix)](https://travis-ci.org/onosproject/onos-test)
[![Go Report Card](https://goreportcard.com/badge/github.com/atomix/k8s-controller)](https://goreportcard.com/report/github.com/atomix/k8s-controller)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://github.com/gojp/goreportcard/blob/master/LICENSE)
[![GoDoc](https://godoc.org/github.com/atomix/k8s-controller?status.svg)](https://godoc.org/github.com/atomix/k8s-controller)

This project provides an [Atomix] controller for [Kubernetes]. The controller
implements the Atomix controller API and uses [custom Kubernetes resources][custom-resources]
to provide seamless integration, allowing standard k8s tools to be used to deploy and scale
partition groups and partitions. For more information see [how it works](#how-it-works).

## Deployment

To deploy the controller, first build the image using the make file:

```bash
> make build
```

The build script will build an `atomix/k8s-controller` image with the `latest`
tag. Once the image has been built, the controller can be deployed to k8s using the
following command:

```bash
> kubectl create -f https://raw.githubusercontent.com/atomix/k8s-controller/master/deploy/atomix-controller.yaml
customresourcedefinition.apiextensions.k8s.io/partitionsets.k8s.atomix.io created
customresourcedefinition.apiextensions.k8s.io/partitions.k8s.atomix.io created
clusterrole.rbac.authorization.k8s.io/atomix-controller created
clusterrolebinding.rbac.authorization.k8s.io/atomix-controller created
serviceaccount/atomix-controller created
deployment.apps/atomix-controller created
service/atomix-controller created
```

The default configuration will deploy a controller named `atomix-controller` in the
`kube-system` namespace. It will also configure the Kubernetes cluter with two custom
resource types: `PartitionSet` and `Partition`. Once the controller has been deployed,
it can be used to create Atomix partitions either using the Atomix client API or through
standard Kubernetes CLI and other APIs.

## Usage

The role of an Atomix controller is to manage partition groups and partitions within a
specific environment. The Kubernetes Atomix controller manages partition groups using
[custom resources][custom-resources]. The controller adds two custom resources to the 
k8s cluster:
* `PartitionSet` defines a set of partitions and the protocol they implement
* `Partition` is used by the `PartitionSet` controller to configure a single partition

Because the k8s controller uses custom resources for partition management, partition groups
and partitions can be managed directly through the k8s API. To add a partition group via the
k8s API, simply define a `PartitionSet` object:

```yaml
apiVersion: k8s.atomix.io/v1alpha1
kind: PartitionSet
metadata:
  name: raft
spec:
  partitions: 6
  template:
    spec:
      size: 3
      protocol: raft
      config: |
        electionTimeout: 5s
        heartbeatInterval: 1s
```

The `PartitionSet` spec requires three fields:
* `partitions` - the number of partitions to create
* `partitionSize` - the number of pods in each partition
* A protocol configuration

The protocol configuration may be one of:
* `raft` - the strongly consistent, persistent [Raft consensus protocol][Raft]
* `backup` - an in-memory primary-backup replication protocol
* `log` - a primary-backup based persistent distributed log replication protocol

The above configuration defines a partition group named `raft` that deploys `6` Raft
partitions with `3` pods in each partition. Each of the six Raft partitions is essentially
an independent 3-node Raft cluster.

To create the partitions, use `kubectl` to create the resource:

```bash
> kubectl create -f raft.yaml
partitionset.k8s.atomix.io/raft created
```

Once the partition group has been created, you should be able to see the partition group 
via `kubectl`:

```bash
> kubectl get partitionsets
NAME   AGE
raft   12s
```

The partition group will create a number of partitions equal to the `partitions` defined
in the partition group spec:

```bash
> kubectl get partitions
NAME     AGE
raft-1   57s
raft-2   57s
raft-3   57s
```

Each partition will create a `StatefulSet`:

```bash
> kubectl get statefulsets
NAME     READY   AGE
raft-1   1/1     2m11s
raft-2   1/1     2m11s
raft-3   1/1     2m11s
```

And each `StatefulSet` contains a number of pods equal to the partition template's `size`:

```bash
> kubectl get pods
NAME       READY   STATUS    RESTARTS   AGE
raft-1-0   1/1     Running   0          74s
raft-1-1   1/1     Running   0          74s
raft-1-2   1/1     Running   0          74s
raft-2-0   1/1     Running   0          74s
raft-2-1   1/1     Running   0          74s
raft-2-2   1/1     Running   0          74s
raft-3-0   1/1     Running   0          74s
raft-3-1   1/1     Running   0          74s
raft-3-2   1/1     Running   0          74s
...
```

A `Service` will be created for each partition in the group as well:

```bash
> kubectl get services
NAME         TYPE        CLUSTER-IP      EXTERNAL-IP   PORT(S)    AGE
raft-1       ClusterIP   10.98.166.215   <none>        5678/TCP   95s
raft-2       ClusterIP   10.109.34.146   <none>        5678/TCP   95s
raft-3       ClusterIP   10.99.37.182    <none>        5678/TCP   95s
...
```

And a partition group service will be created as well, allowing partitions
to be resolved over DNS using SRV records:

```bash
> kubectl get services
NAME         TYPE        CLUSTER-IP      EXTERNAL-IP   PORT(S)    AGE
raft         ClusterIP   10.97.241.64    <none>        5678/TCP   96s
...
> kubectl get endpoints
NAME         ENDPOINTS                                                 AGE
raft         10.109.34.146:5678,10.98.166.215:5678,10.99.37.182:5678   120s
...
```

Once the partition group is deployed and ready, it can used to create and operate on 
distributed primitives programmatically using any [Atomix client][atomix-go-client]:

```go
import (
	atomixclient "github.com/atomix/go-client/pkg/client"
)

client, err := atomixclient.New("atomix-controller.kube-system.svc.cluster.local:5679")
if err != nil {
	...
}

group, err := client.GetGroup(context.TODO(), "raft")
if err != nil {
	...
}

lock, err := group.GetLock(context.TODO(), "my-lock")
if err != nil {
	...
}

id, err := lock.Lock(context.TODO())
if err != nil {
	...
}
```

## How it works

Atomix 4 provides a framework for building, running and scaling replicated state machines
using a variety of protocols (e.g. Raft consensus, primary-backup, distributed log, etc) and
exports a gRPC API for operating on replicated state machines. Atomix primitives are designed
to scale through partitioning, but the Atomix core framework does not handle partitioning itself.
Instead, it exports a gRPC API for implementing Atomix controllers and leaves partition
management to environment-specific controller implementations like this one.

The Atomix k8s controller implements the Atomix 4 controller API and runs inside Kubernetes to
manage deployment of partition groups and partitions using
[custom resource controllers][custom-resources].

![Kubernetes Controller Architecture](https://i.imgur.com/krk3y00.png)

Partition groups and partitions can be managed either through the
[Atomix client API][atomix-go-client] or using standard Kubernetes tools like `kubectl`.

The Atomix controller provides two custom Kubernetes controllers for managing partition
groups and partitions. When a partition group is created via the Atomix controller API,
the Atomix controller creates a `PartitionSet` resource, and the remainder of the deployment
process is managed by custom Kubernetes controllers. The `PartitionSet` controller
creates a set of `Partition` resources from the `PartitionSet`, and the `Partition`
controller creates `StatefulSet` resources, `Service`s, and other resources necessary to
run the protocol specified by the `PartitionSet` configuration.

![Custom Resources](https://i.imgur.com/mbiTCI6.png)

### The control loop

When a `PartitionSet` is created, the k8s partition group controller will create a
`Service` for the partition group:

```bash
> kubectl get services
NAME         TYPE        CLUSTER-IP      EXTERNAL-IP   PORT(S)    AGE
raft         ClusterIP   10.97.241.64    <none>        5678/TCP   96s
...
```

The partition group service is a special service used to resolve the partitions in the
group via DNS using SRV records, much like pod IPs are resolved for headless services.

When a `PartitionSet` is created, the k8s partition group controller will create a
`Partition` resource for each partition specified in the group's spec:

```bash
> kubectl get partitions
NAME     AGE
raft-1   57s
raft-2   57s
raft-3   57s
```

Partition names are prefixed with the owner group's name, and partitions are annotated
with information about the group and the controller as well:

```bash
> kubectl describe partition raft-1
Name:         raft-1
Namespace:    default
Annotations:  k8s.atomix.io/controller: kube-system.atomix-controller
              k8s.atomix.io/group: raft
              k8s.atomix.io/partition: 1
              k8s.atomix.io/type: partition
API Version:  k8s.atomix.io/v1alpha1
Kind:         Partition
...
```

When a `Partition` is created, the k8s partition controller will create a number of
resources for deploying the partition pods, establishing connectivity between them,
and accessing them from other pods in the cluster.

First, the partition controller creates a `StatefulSet` for the partition:

```bash
> kubectl get statefulsets
NAME     READY   AGE
raft-1   1/1     2m11s
...
```

And, of course, the `StatefulSet` controller creates `Pod`s for each node in the partition:

```bash
> kubectl get pods
NAME       READY   STATUS    RESTARTS   AGE
raft-1-0   1/1     Running   0          74s
raft-1-1   1/1     Running   0          74s
raft-1-2   1/1     Running   0          74s
...
```

Then, the controller creates two `Service`s for the partition's `StatefulSet`: a
headless service used by replication protocols to form a cluster, and a normal
service used by other pods to access the partition:

```bash
> kubectl get services
NAME         TYPE        CLUSTER-IP      EXTERNAL-IP   PORT(S)    AGE
raft-1       ClusterIP   10.98.166.215   <none>        5678/TCP   95s
raft-1-hs    ClusterIP   None            <none>        5678/TCP   95s
...
```

As the services for the partition are created, the parent partition group's `Service`
is updated with `Endpoints` that resolve the service to the partition service names:

```bash
> kubectl get endpoints
NAME         ENDPOINTS                                                 AGE
raft         10.109.34.146:5678,10.98.166.215:5678,10.99.37.182:5678   120s
...
```

This allows clients to resolve the partitions within a group via SRV records without
even connecting to the Atomix controller.

[Atomix]: https://atomix.io
[Kubernetes]: https://kubernetes.io
[custom-resources]: https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/
[atomix-go-client]:https://github.com/atomix/go-client
[Raft]: https://raft.github.io/
