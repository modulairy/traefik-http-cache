# Development and local loading

Run all commands below from the repository root.

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

Original project code is licensed under [Apache License 2.0](../LICENSE). See [NOTICE](../NOTICE) and [third-party notices](../THIRD_PARTY_NOTICES.md). Bundled Traefik, the Go runtime and base-image components retain their own licenses.
