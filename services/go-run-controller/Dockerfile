FROM golang:1.26.8-alpine3.24 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w' -o /out/go-run-controller ./cmd/go-run-controller

FROM alpine:3.24
RUN addgroup -g 65532 -S runctl && adduser -u 65532 -S -G runctl -H -s /sbin/nologin runctl \
 && mkdir -p /var/lib/go-run-controller && chown 65532:65532 /var/lib/go-run-controller
COPY --from=build /out/go-run-controller /usr/local/bin/go-run-controller
USER 65532:65532
EXPOSE 8080
ENV LISTEN_ADDR=:8080 \
    RUN_DATA_DIR=/var/lib/go-run-controller/data \
    RUN_EVENTS_FILE=/var/lib/go-run-controller/events.jsonl \
    RUN_OUTBOX_FILE=/var/lib/go-run-controller/outbox.jsonl
ENTRYPOINT ["/usr/local/bin/go-run-controller"]
