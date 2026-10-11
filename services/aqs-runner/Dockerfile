FROM golang:1.26.9-alpine3.24 AS build
WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w' -o /out/aqs-runner ./cmd/aqs-runner

FROM alpine:3.24
RUN addgroup -g 65532 -S runner && adduser -u 65532 -S -G runner -H -s /sbin/nologin runner
COPY --from=build /out/aqs-runner /usr/local/bin/aqs-runner
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/aqs-runner"]
