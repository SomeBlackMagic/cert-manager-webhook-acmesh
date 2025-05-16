FROM golang:1.16-alpine AS build
RUN apk add --no-cache git curl ca-certificates socat bash openssl
WORKDIR /workspace
COPY go.mod .
COPY go.sum .
RUN go mod download
COPY . .
RUN curl -fsSL https://get.acme.sh | sh
RUN CGO_ENABLED=0 go build -o webhook -ldflags '-w -extldflags "-static"' .

FROM alpine
WORKDIR /root
COPY --from=build /workspace/webhook /usr/local/bin/webhook
COPY --from=build /root/.acme.sh /root/.acme.sh
ADD acme_delegate.sh /root/acme_delegate.sh
RUN apk add --no-cache ca-certificates curl socat bash openssl && chmod 755 /root/acme_delegate.sh

ENTRYPOINT ["webhook"]