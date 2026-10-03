FROM golang:1.26.8-alpine3.24 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w' -o /out/go-governance ./cmd/go-governance

FROM alpine:3.24
RUN addgroup -g 65532 -S governance && adduser -u 65532 -S -G governance -H -s /sbin/nologin governance \
 && mkdir /data && chown 65532:65532 /data
COPY --from=build /out/go-governance /usr/local/bin/go-governance
USER 65532:65532
EXPOSE 8080
ENV LISTEN_ADDR=:8080 GOVERNANCE_DATA_DIR=/data
ENTRYPOINT ["/usr/local/bin/go-governance"]
