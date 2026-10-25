# Traefik HTTP Cache

Rule-based HTTP response caching for Traefik, with interchangeable memory and Redis stores. The first release is **v0.1.0**.

The recommended installation uses Traefik's remote plugin loader with a pinned module version. The plugin runs inside Traefik through Yaegi; it is not a separate cache service. Dynamic middleware configuration selects rules and storage.

## Install in an existing Traefik

Once remote distribution passes the availability check below, add this to the existing static configuration, then restart Traefik:

```yaml
experimental:
  plugins:
    httpCache:
      moduleName: github.com/modulairy/traefik-http-cache
      version: v0.1.0
```

Remote mode downloads plugin source at startup and requires access to Traefik's plugin distribution endpoints. A compatible existing Traefik image can be retained; no custom image or init container is required. Configure the middleware below and attach it to the desired router. See the [official installation guide](https://plugins.traefik.io/install).

**Remote availability check — 2026-10-25:** a fresh Traefik v3.7.16 startup returned `404 Unknown plugin` for this module at `v0.1.0`, disabled plugins and invalidated dependent routers. Remote installation is recommended once catalog distribution is available; it is not ready as of this check. Use local mode below meanwhile. The repository is public, is not a fork and has the required topic; await discovery and inspect ingestion issues. See the [catalog checklist](docs/releases.md#plugin-catalog).

## Features

- Path matching: exact, prefix, contains, and precompiled regex.
- Request and response header conditions; Content-Type filtering.
- Conservative public GET response caching with Cache-Control handling.
- ETag and Last-Modified conditional revalidation, on expiry or every use.
- Same-URI invalidation after successful unsafe requests, with generation checks to reject racing writes.
- Bounded per-middleware memory store or shared standalone Redis, including TLS and password files.

## Try locally

For development or offline plugin loading, the supplied container bundles Traefik and the local plugin source; `infra/traefik.yml` enables `experimental.localPlugins.httpCache`. Use either remote or local registration for this alias. Docker Engine and Docker Compose are required. From the repository root:

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

With GNU Make and a POSIX shell, `make check` runs formatting verification, vet and race tests; `make up`, `make integration` and `make down` wrap the Compose commands. Wait for router readiness before `make integration`. `make fmt` formats the Go source.

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

## Project layout and license

The Traefik entry point remains a root package with `Config`, `CreateConfig` and `New`, following the [official plugin example](https://github.com/traefik/plugindemo). Storage is a supporting package under `internal/store`, following the [Go module layout guidance](https://go.dev/doc/modules/layout#package-or-command-with-supporting-packages). This keeps adapter implementation details out of the public plugin API.

```text
.
├── .traefik.yml, go.mod, Makefile
├── http_cache.go, config.go, response.go  # Traefik entry point and HTTP policy
├── *_test.go                            # HTTP and integration tests
├── internal/
│   ├── store/                           # Store contract, memory/Redis adapters and tests
│   └── testbackend/                     # Integration fixture
├── container/                           # Container build definitions
├── infra/                               # Compose and Kubernetes examples
└── docs/                                # Configuration, design and releases
```

Original project code is licensed under [Apache License 2.0](LICENSE). See [NOTICE](NOTICE) and [third-party notices](THIRD_PARTY_NOTICES.md). Bundled Traefik, the Go runtime and base-image components retain their own licenses.
