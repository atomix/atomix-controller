FROM alpine:3.8

RUN apk upgrade --update --no-cache

USER nobody

ADD bin/atomix-controller-init-certs /usr/local/bin/atomix-controller-init-certs

ENTRYPOINT ["atomix-controller-init-certs"]
