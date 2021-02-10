FROM alpine:3.8

RUN apk upgrade --update --no-cache

USER nobody

ADD build/coordinator/_output/bin/kubernetes-coordinator /usr/local/bin/atomix-coordinator

ENTRYPOINT ["atomix-coordinator"]
