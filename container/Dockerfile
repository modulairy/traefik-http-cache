ARG TRAEFIK_VERSION=v3.7.16@sha256:c8a121e4bb1347e8fd52657fab12f2cb70b68f8a018895e2068f92c9e519b9c2
FROM traefik:${TRAEFIK_VERSION}
ARG VERSION=dev
ARG REVISION=unknown
LABEL org.opencontainers.image.source="https://github.com/modulairy/traefik-http-cache" \
      org.opencontainers.image.title="Traefik HTTP Cache" \
      org.opencontainers.image.description="Traefik with the HTTP cache local plugin bundled" \
      org.opencontainers.image.licenses="Apache-2.0 AND MIT AND BSD-3-Clause" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${REVISION}"
WORKDIR /
COPY go.mod .traefik.yml http_cache.go config.go redis.go response.go store.go LICENSE NOTICE /plugins-local/src/github.com/modulairy/traefik-http-cache/
COPY LICENSE NOTICE THIRD_PARTY_NOTICES.md /usr/share/licenses/traefik-http-cache/
COPY licenses/ /usr/share/licenses/traefik-http-cache/third-party/
# Enable the bundled plugin in static configuration (see infra/traefik.yml).
