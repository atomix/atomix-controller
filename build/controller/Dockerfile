FROM alpine:3.8

RUN apk upgrade --update --no-cache

USER nobody

ADD build/controller/_output/bin/k8s-controller /usr/local/bin/atomix-controller
