FROM golang:1.26.9-alpine3.24 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w' -o /out/go-intake ./cmd/go-intake

FROM alpine:3.24
RUN addgroup -g 65532 -S intake && adduser -u 65532 -S -G intake -H -s /sbin/nologin intake \
 && mkdir -p /var/lib/go-intake && chown 65532:65532 /var/lib/go-intake
COPY --from=build /out/go-intake /usr/local/bin/go-intake
USER 65532:65532
EXPOSE 8080
ENV LISTEN_ADDR=:8080 \
    INTAKE_DATA_DIR=/var/lib/go-intake/data \
    INTAKE_EVENTS_FILE=/var/lib/go-intake/events.jsonl
ENTRYPOINT ["/usr/local/bin/go-intake"]
