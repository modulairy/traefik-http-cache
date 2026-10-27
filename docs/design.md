# Design and operational boundaries

The middleware owns HTTP policy; stores own persistence. There are no external Go dependencies. The Redis adapter implements only the required RESP2 subset to keep interpreted plugin dependencies small.

## Source layout

The repository keeps the root Traefik entry point from the [official plugin example](https://github.com/traefik/plugindemo): `Config`, `CreateConfig` and `New`, with root `go.mod`, `.traefik.yml`, HTTP tests and `Makefile`. `http_cache.go` handles HTTP orchestration, `config.go` validates the public configuration and rules, and `response.go` captures and forwards responses.

Storage follows the [Go recommendation for supporting packages](https://go.dev/doc/modules/layout#package-or-command-with-supporting-packages): `internal/store` contains `store.go` (the generation-aware contract), `memory.go`, `redis.go`, adapter-specific `config.go`, and colocated store tests. The HTTP package maps public configuration to the internal adapters; the store package handles opaque values and does not depend on HTTP policy. Its API can evolve without becoming an externally supported package.

`internal/testbackend` remains an integration fixture, not the plugin entry point. `container/` and `infra/` package and exercise the plugin; `docs/` explains its behavior. The container must include both root plugin source and `internal/store` so Yaegi can load the supporting package. Real Traefik integration tests remain the compatibility check for this layout.

This is a layout/API convention, not a claim of Traefik certification or plugin catalog approval. The original project is Apache-2.0; upstream components retain their own licenses as described in `THIRD_PARTY_NOTICES.md`.

1. Successful unsafe requests invalidate their exact scheme/host/path/query scope.
2. Other requests are checked for bypass conditions and matched to the first request rule.
3. The store returns the representation and its URI generation.
4. Fresh entries are served with an updated Age; stale entries are conditionally validated where validators exist.
5. A 304 refreshes metadata and reuses the stored body. A replacement 200 is stored only if all response policy checks pass.
6. Store writes carry the generation read earlier. Invalidation changes/removes that generation, preventing an older in-flight fill from restoring stale data.

Cache identity includes an application prefix, request scheme/host/URI, rule configuration fingerprint, rule name, method and explicitly selected request header values. Configured `varyHeaders` are always added to the effective key before contacting the origin; currently only opt-in Accept-Encoding is supported. Presence and raw value arrays distinguish absent, empty, qweighted and multiline values without a variant index. Opt-in keys have a separate format namespace; configuration fingerprints prevent incompatible policy reuse. HTTP semantics stay in the middleware; stores still receive opaque serialized bytes. Redis shares data only when instances use matching identity and database settings. TLS termination outside Traefik is not inferred from arbitrary forwarded headers; scope uses the request's actual TLS state.

## Revalidation

Freshness and physical retention are separate. A retained stale body can be reused only after validation; `always` validates every reuse. ETag is sent in If-None-Match, and Last-Modified in If-Modified-Since when available. Without validators the origin returns a complete representation. Backend failures are forwarded, never replaced with stale content. A 304 with conflicting ETag triggers an unconditional fetch.

Before 304 body reuse, an explicitly supplied Vary set must match the stored set and the rule allowlist. Wildcard, unsupported or changed sets and changed Content-Encoding invalidate the exact URI, including all its encoding variants, and cause one unconditional origin request. A missing Vary or Content-Encoding on 304 retains stored metadata. The fallback response is forwarded without cache storage; the next eligible request may populate a new generation. Gzip bodies are never transformed. The opt-in is a conservative Accept-Encoding implementation, not general Vary negotiation or full RFC 9111 conformance.

## Invalidation and consistency

Successful 2xx/3xx unsafe methods invalidate the same URI, including its configured header variants. Query variants are different URIs. Related lists, other hosts, redirects, Location/Content-Location targets and changes that bypass this middleware are not automatically invalidated. Cross-resource tags, webhooks and purge APIs are deferred.

Generation checks prevent late fills after invalidation; they do not provide linearizable reads. A request already reading an entry can finish while a write invalidates it. Redis read errors bypass to the origin. Failed invalidation disables caching locally for the maximum retention duration, but cannot notify other Traefik instances: those instances may retain stale data until freshness expires or a successful invalidation occurs. Use short freshness periods or `always` where this matters. No automatic Redis-to-memory failover exists.

## Limits and deferred work

- Memory is per middleware instance, not a shared process-wide service. Expiry cleanup scans bounded maps during operations.
- Redis supports standalone servers only. There is no connection pool or request coalescing, so simultaneous misses may all reach the origin.
- All cached bodies are buffered up to the configured limit and serialized; cache capacity and per-response size require workload-specific tuning.
- Private/user-specific caching, Vary fields other than opt-in Accept-Encoding, range caching, client conditional cache responses, stale serving, distributed purge events and background refresh are excluded.
- Middleware order matters. Validate interaction with authentication, compression and response-mutating middleware before deployment. Only public endpoints belong in this cache.

## Validation

Native Go tests exercise matching, cache policy, expiry, revalidation, bounded stores and invalidation. Race tests and vet check native code. Compose tests separately exercise Redis commands and actual Traefik plugin loading through Yaegi. Passing native compilation alone is not evidence of interpreter compatibility. Kubernetes manifests are integration examples, not an installed cluster or a claim of cluster validation.
