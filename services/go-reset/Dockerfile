FROM golang:1.26.9-alpine3.24 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w' -o /out/go-reset ./cmd/go-reset

FROM alpine:3.24
RUN addgroup -g 65532 -S reset && adduser -u 65532 -S -G reset -H -s /sbin/nologin reset
COPY --from=build /out/go-reset /usr/local/bin/go-reset
USER 65532:65532
EXPOSE 8080
ENV LISTEN_ADDR=:8080
ENTRYPOINT ["/usr/local/bin/go-reset"]
