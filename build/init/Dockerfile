FROM alpine:3.8

RUN apk upgrade --update --no-cache

USER nobody

ADD build/atomix-controller-init/_output/bin/atomix-controller-init /usr/local/bin/atomix-controller-init

ENTRYPOINT ["atomix-controller-init"]
