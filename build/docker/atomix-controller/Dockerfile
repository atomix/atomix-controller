FROM alpine:3.8

RUN apk upgrade --update --no-cache

USER nobody

ADD bin/atomix-controller /usr/local/bin/atomix-controller

ENTRYPOINT ["atomix-controller"]
