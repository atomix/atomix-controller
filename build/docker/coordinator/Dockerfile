FROM alpine:3.8

RUN apk upgrade --update --no-cache

USER nobody

ADD bin/atomix-coordinator /usr/local/bin/atomix-coordinator

ENTRYPOINT ["atomix-coordinator"]
