FROM golang:1.26.8-alpine3.24 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w' -o /out/ui-api ./cmd/ui-api

FROM alpine:3.24
RUN addgroup -g 65532 -S uiapi && adduser -u 65532 -S -G uiapi -H -s /sbin/nologin uiapi \
 && mkdir -p /var/lib/ui-api && chown 65532:65532 /var/lib/ui-api
COPY --from=build /out/ui-api /usr/local/bin/ui-api
USER 65532:65532
EXPOSE 8080
ENV LISTEN_ADDR=:8080 \
    UIAPI_DATA_DIR=/var/lib/ui-api/data \
    UIAPI_EVENTS_FILE=/var/lib/ui-api/events.jsonl \
    UIAPI_OUTBOX_FILE=/var/lib/ui-api/outbox.jsonl
ENTRYPOINT ["/usr/local/bin/ui-api"]
