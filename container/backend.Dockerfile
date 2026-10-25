FROM golang:1.27.3@sha256:f20d83a83e18dbd2a4714b8ffd4b30b287b6243962dd75608e86abcc63ba4efc AS build
WORKDIR /src
COPY go.mod ./
COPY internal/testbackend ./internal/testbackend
RUN CGO_ENABLED=0 go build -trimpath -o /backend ./internal/testbackend
FROM scratch
COPY --from=build /backend /backend
COPY LICENSE NOTICE /usr/share/licenses/traefik-http-cache/
COPY licenses/go-LICENSE /usr/share/licenses/go/LICENSE
USER 65532:65532
ENTRYPOINT ["/backend"]
