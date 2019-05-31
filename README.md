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
`kube-system` namespace.__

[Atomix]: https://atomix.io
[Kubernetes]: https://kubernetes.io
