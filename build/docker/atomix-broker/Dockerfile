FROM alpine:3.8

RUN apk upgrade --update --no-cache

USER nobody

ADD bin/atomix-broker /usr/local/bin/atomix-broker

ENTRYPOINT ["atomix-broker"]
