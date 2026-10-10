FROM golang:1.26.9-alpine3.24 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w' -o /out/go-warm-manager ./cmd/go-warm-manager

FROM alpine:3.24
RUN addgroup -g 65532 -S warm && adduser -u 65532 -S -G warm -H -s /sbin/nologin warm
COPY --from=build /out/go-warm-manager /usr/local/bin/go-warm-manager
USER 65532:65532
EXPOSE 8080
ENV LISTEN_ADDR=:8080 WARM_NAMESPACE=aqs-test WARM_OUTBOX_FILE=/tmp/outbox.jsonl
ENTRYPOINT ["/usr/local/bin/go-warm-manager"]
