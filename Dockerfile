FROM golang:1.16-alpine AS build
WORKDIR /workspace
COPY go.mod .
COPY go.sum .
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o webhook -ldflags '-w -extldflags "-static"' .

FROM neilpang/acme.sh

RUN apk add bash

ARG DOCKER_APP_UID="1000"
ARG DOCKER_APP_GID="1000"

####
#### User/Group
####

RUN set -eux \
    && addgroup -g ${DOCKER_APP_GID} app \
    && adduser -u ${DOCKER_APP_UID} -G app -s /bin/sh -D app \
    && true


RUN set -eux \
    && mv /root/.acme.sh /opt/.acme.sh \
    && chown -R app:app /opt/.acme.sh \
    && chown -R app:app /acme.sh \
    && true


COPY --from=build --chown=app:app --chmod=777 /workspace/webhook /usr/local/bin/webhook
COPY --chown=app:app --chmod=777 acme_delegate.sh /usr/local/bin/acme_delegate.sh


USER app

ENTRYPOINT ["/usr/local/bin/webhook"]
CMD [""]