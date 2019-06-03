# Atomix Kubernetes Controller

This project provides an [Atomix] controller for [Kubernetes]. The controller is implemented
as a k8s controller, using custom resources to manage partition groups and partitions.

## Deployment

To deploy the controller, first build the image using the make file:

```bash
> make build
```

The build script will build an `atomix/atomix-k8s-controller` image with the `latest`
tag. Once the image has been built, the controller can be deployed to k8s using the
following sequence of commands:

```bash
> kubectl create -f deploy/atomix-controller.yaml
```

The default configuration will deploy a controller named `atomix-controller` in the
`kube-system` namespace. Once the controller has been deployed, it can be used to create
Atomix partitions using the Atomix client API or through the Kubernetes API.

## Usage

The role of an Atomix controller is to manage partition groups and partitions within a
specific environment. The Kubernetes Atomix controller manages partition groups using
[custom resources][custom-resources]. The controller adds two custom to the k8s cluster:
* `PartitionGroup` defines a set of partitions and the protocol they implement
* `Partition` is used by the `PartitionGroup` controller to configure a single partition

Because the k8s controller uses custom resources for partition management, partition groups
and partitions can be managed directly through the k8s API. To add a partition group via the
k8s API, simply define a `PartitionGroup` object:

```yaml
apiVersion: k8s.atomix.io/v1alpha1
kind: PartitionGroup
metadata:
  name: raft
spec:
  version: latest
  partitions: 3
  partitionSize: 3
  raft: {}
```

This configuration defines a partition group that runs the `raft` protocol in `3`
partitions with `3` pods in each partition. To create the partitions, use the k8s cli
to create the partition group:

```bash
> kubectl create -f raft.yaml
partitiongroup.k8s.atomix.io/raft created
```

Once the partition group has been created, you can see the partition group via `kubectl`:

```bash
> kubectl get partitiongroups
NAME   AGE
raft   12s
```

Internally, the `PartitionGroup` controller will create a `Partition` resource for each
partition in the group:

```bash
> kubectl get partitions
NAME     AGE
raft-1   57s
raft-2   57s
raft-3   57s
```

And the `Partition` controller will create `StatefulSet`s and `Pod`s to run the partitions,
and `Service`s through which the partitions can be accessed:

```bash
> kubectl get statefulsets
NAME     READY   AGE
raft-1   1/1     2m11s
raft-2   1/1     2m11s
raft-3   1/1     2m11s
> kubectl get pods
NAME       READY   STATUS    RESTARTS   AGE
raft-1-0   1/1     Running   0          2m14s
raft-2-0   1/1     Running   0          2m14s
raft-3-0   1/1     Running   0          2m14s
> kubectl get services
NAME         TYPE        CLUSTER-IP   EXTERNAL-IP   PORT(S)    AGE
kubernetes   ClusterIP   10.96.0.1    <none>        443/TCP    16d
raft-1       ClusterIP   None         <none>        5678/TCP   2m16s
raft-1-hs    ClusterIP   None         <none>        5678/TCP   2m16s
raft-2       ClusterIP   None         <none>        5678/TCP   2m16s
raft-2-hs    ClusterIP   None         <none>        5678/TCP   2m16s
raft-3       ClusterIP   None         <none>        5678/TCP   2m16s
raft-3-hs    ClusterIP   None         <none>        5678/TCP   2m16s
```

The controller can also be used programmatically via Atomix client libraries:

```go
controller, err := client.NewClient("atomix-controller.kube-system.svc.cluster.local:5679")
if err != nil {
	...
}

group, err := controller.CreatePartitionGroup("raft", 3, 3, raft.Protocol{})
if err != nil {
	...
}
```

## How it works

Atomix 4 provides a framework for building and running and scaling replicated state machines
using a variety of protocols (e.g. Raft consensus, primary-backup, distributed log, etc) and
exports a gRPC API for operating on replicated state machines. Atomix primitives are designed
to scale through partitioning, but the Atomix core framework does not handle partitioning itself.
Instead, it exports a gRPC API for implementing Atomix controllers and leaves partition
management to environment-specific controller implementations like this one.

The Atomix k8s controller implements the Atomix 4 controller API and runs inside Kubernetes to
manage deployment of partition groups and partitions using
[custom resource controllers][custom-resources].

![Kubernetes Controller Architecture](https://i.imgur.com/krk3y00.png)

Clients can access the k8s controller through the controller's service, allowing
Atomix clients to locate and manage partition groups within the Kubernetes cluster
using the Atomix controller API:

```go
controller, err := client.NewClient("atomix-controller.kube-system.svc.cluster.local:5679")
if err != nil {
	...
}

group, err := controller.GetPartitionGroup("raft")
if err != nil {
	...
}

counter, err := group.NewCounter("my-counter")
if err != nil {
	...
}

count, err := counter.Increment(context.Background(), 1)
```

Additionally, the k8s controller creates SRV records for each partition group that resolve
to a list of the service IPs for each partition. This allows clients to connect to a partition
group by simply connecting to the group's service, fetching partition locations via DNS:

```go
// Connect to a partition group named "raft"
client, err := group.NewClient("raft")
if err != nil {
	...
}

lock, err := client.NewLock("my-lock")
if err != nil {
	...
}

id, err := lock.Lock(context.Background())
```

The Atomix k8s controller provides two custom Kubernetes controllers for managing partition
groups and partitions. When a partition group is created via the Atomix controller API,
the k8s controller creates a `PartitionGroup` resource, and the remainder of the deployment
process is managed by custom Kubernetes controllers. The custom `PartitionGroup` controller
creates a set of `Partition` resources from the `PartitionGroup`, and the custom `Partition`
controller creates `StatefulSet` resources and `Service`s to run the protocol specified by
the `Partition` configuration.

![Custom Resources](https://i.imgur.com/mbiTCI6.png)

[Atomix]: https://atomix.io
[Kubernetes]: https://kubernetes.io
[custom-resources]: https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/
[atomix-go-client]:https://github.com/atomix/atomix-go-client
