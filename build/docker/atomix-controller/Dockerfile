# SPDX-FileCopyrightText: 2019-present Open Networking Foundation <info@opennetworking.org>
#
# SPDX-License-Identifier: Apache-2.0

FROM alpine:3.8

RUN apk upgrade --update --no-cache

USER nobody

ADD bin/atomix-controller /usr/local/bin/atomix-controller

ENTRYPOINT ["atomix-controller"]
