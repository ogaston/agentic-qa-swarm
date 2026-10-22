# App de referencia del warm (U8-T04). Contexto = demo/target-app/. Sin latest: el tag se fija al construir.
FROM golang:1.26.10-alpine3.24 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w' -o /out/target-app .

FROM alpine:3.24
# Usuario no root. cap_net_bind_service (fichero) permite escuchar en :80, el puerto nombrado `http` del warm-app.
RUN apk add --no-cache libcap-utils \
 && addgroup -g 65532 -S app && adduser -u 65532 -S -G app -H -s /sbin/nologin app
COPY --from=build /out/target-app /usr/local/bin/target-app
RUN setcap cap_net_bind_service=+ep /usr/local/bin/target-app \
 && apk del libcap-utils
USER 65532:65532
EXPOSE 80 9090
ENV LISTEN_ADDR=:80 BASELINE_LISTEN_ADDR=:9090
ENTRYPOINT ["/usr/local/bin/target-app"]
