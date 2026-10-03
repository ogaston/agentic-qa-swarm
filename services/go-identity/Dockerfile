FROM golang:1.24.13-alpine3.22 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w' -o /out/go-identity ./cmd/go-identity

FROM alpine:3.22
RUN addgroup -g 65532 -S identity && adduser -u 65532 -S -G identity -H -s /sbin/nologin identity
COPY --from=build /out/go-identity /usr/local/bin/go-identity
USER 65532:65532
EXPOSE 8080
ENV LISTEN_ADDR=:8080
ENTRYPOINT ["/usr/local/bin/go-identity"]
