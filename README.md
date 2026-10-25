# Traefik HTTP Cache

Rule-based HTTP response caching for Traefik, with interchangeable memory and Redis stores. The first release is **v0.1.0**.

The container bundles Traefik and this local Go plugin. It is not a standalone cache server. Static configuration must enable `experimental.localPlugins.httpCache`; dynamic middleware configuration selects rules and storage.

## Features

- Path matching: exact, prefix, contains, and precompiled regex.
- Request and response header conditions; Content-Type filtering.
- Conservative public GET response caching with Cache-Control handling.
- ETag and Last-Modified conditional revalidation, on expiry or every use.
- Same-URI invalidation after successful unsafe requests, with generation checks to reject racing writes.
- Bounded per-middleware memory store or shared standalone Redis, including TLS and password files.

## Try locally

Docker Engine and Docker Compose are required. From the repository root:

```sh
docker compose -f infra/compose.yml up -d --build
curl -i -H 'Host: memory.localhost' http://localhost:18080/public
curl -i -H 'Host: memory.localhost' http://localhost:18080/public
curl -i -H 'Host: redis.localhost' http://localhost:18080/public
```

The second identical memory request should report `X-Cache-Status: HIT`. The first response reports `MISS`. Backend startup can take a few seconds. The demo exposes only localhost port 18080; its unauthenticated Redis is internal to the Compose network.

```sh
go test -race -count=1 ./...
go vet ./...
docker compose -f infra/compose.yml --profile test run --rm test
docker compose -f infra/compose.yml down --remove-orphans
```

Unit tests skip live tests unless `REDIS_ADDR` / `CACHE_TEST_URL` are set. The Compose test service sets both and exercises Redis plus the actual Traefik/Yaegi runtime. Start the stack and wait for HTTP readiness first.

## Configure

See [configuration](docs/configuration.md), [architecture and limitations](docs/design.md), [Kubernetes examples](infra/kubernetes/README.md), and [release process](docs/releases.md). A complete runnable example is in [infra/dynamic.yml](infra/dynamic.yml).

```yaml
http:
  middlewares:
    catalog-cache:
      plugin:
        httpCache:
          keyPrefix: catalog-public-v1
          store:
            type: memory
            memory:
              maxEntries: 10000
              maxBytes: 67108864
          rules:
            - name: product-json
              request:
                path: {type: regex, value: '^/api/products(/[0-9]+)?$'}
                methods: [GET]
              response:
                contentTypes: [application/json, 'application/*+json']
                headers:
                  - {name: X-Cache-Enabled, operator: exact, value: 'true'}
              cache:
                ttl: 60s
                retention: 10m
                maxBodyBytes: 1048576
                revalidation: {mode: on-expiry}
```

Use a unique `keyPrefix` per application/security boundary. Rules are evaluated in order: the first matching request rule wins. Response conditions control storage, not rule fallback. Only explicitly public, non-personalized endpoints should be enabled.

## Releases

Version tags trigger validation, then publish `ghcr.io/modulairy/traefik-http-cache:<tag>` and a GitHub Release. For the initial release the tag is `v0.1.0`. Publication status is determined by the Actions run; source availability alone does not imply an image has been published. The image targets Linux AMD64.

No license has been selected yet. This repository does not currently grant an open-source license.
