# Configuration

The plugin alias is `httpCache`, module `github.com/modulairy/traefik-http-cache`. **As of 2026-10-25, a fresh Traefik v3.7.16 remote startup returned `404 Unknown plugin` at `v0.1.0`; plugins were disabled and dependent routers were invalidated. Use local mode until distribution is available.** The following is the recommended registration once remote availability is confirmed:

```yaml
experimental:
  plugins:
    httpCache:
      moduleName: github.com/modulairy/traefik-http-cache
      version: v0.1.0
```

This belongs to Traefik's static configuration. Restart Traefik after adding it or changing the pinned version; dynamic middleware rules use the same `httpCache` alias. Source is downloaded during startup and interpreted inside the Traefik process, so the existing compatible Traefik image does not need replacement. Remote startup needs network access to plugin distribution services and a writable `./plugins-storage` directory relative to the Traefik working directory. With a read-only container filesystem, mount a dedicated writable volume there and verify runtime UID permissions. Verify successful loading in logs before routing traffic through the middleware. [Official installation guide](https://plugins.traefik.io/install).

For local development or offline plugin loading, use the bundled image and `experimental.localPlugins` configuration in `infra/traefik.yml` instead. Local mode reads source under `/plugins-local/src/github.com/modulairy/traefik-http-cache`, including `internal/store`; it does not download the plugin. Do not register the same alias in both modes. The repository's Compose environment validates local mode; it does not prove catalog listing or remote download availability.

## Rules

`keyPrefix` is required. `rules` must contain at least one rule with a unique nonempty `name`.

| Field | Behavior/default |
| --- | --- |
| `request.path.type` | Required: `exact`, `prefix`, `contains`, `regex` |
| `request.path.value` | Required path expression; regex uses Go regexp syntax |
| `request.methods` | `[GET]`; other methods cannot be configured for caching |
| `request.headers` / `response.headers` | List of `name`, `operator`, `value`; operators: `exists`, `exact`, `prefix`, `contains`, `regex` |
| `response.contentTypes` | Optional alternatives; exact MIME types or `application/*+json` |
| `cache.ttl` | `60s`; positive duration, upper bound on freshness |
| `cache.retention` | `10m`; at least TTL, at most `24h` |
| `cache.maxBodyBytes` | `1048576`; allowed range 1–67108864 |
| `cache.keyHeaders` | Optional request header names that distinguish representations |
| `cache.revalidation.mode` | `on-expiry` or `always`; default `on-expiry` |

Conditions within a rule are ANDed; Content-Type alternatives are ORed. Matching is case-sensitive except header names and MIME comparison. MIME parameters on the response are ignored. Regex is compiled once at middleware construction; use anchors for full-path matching. The decoded URL path is matched, while the original escaped path and raw query participate in the cache identity. Query parameters are not reordered or dropped.

Header values are joined with `, ` before matching. `exists` checks field presence, including an empty value. A request header condition does not automatically add that header to the cache key. Add every origin representation selector to `keyHeaders`, or do not cache that endpoint. `Vary` responses are conservatively excluded even when key headers are configured.

## Stores

Memory is the default store. Defaults are 10000 entries and 64 MiB serialized value bytes. These limits do not represent total Go heap usage: keys, maps and object overhead also consume memory. Each middleware construction gets an independent store, including inside the same Traefik process. Restarts and reloads discard it. Cleanup is lazy during cache operations; there is no background worker or strict LRU policy. Capacity can cause writes to be skipped or URI scopes to be evicted.

Redis configuration:

```yaml
store:
  type: redis
  redis:
    address: redis.cache.svc.cluster.local:6379
    db: 0
    timeout: 200ms
    tls: true
    serverName: redis.cache.svc.cluster.local
    username: cache
    passwordFile: /run/secrets/redis-password
```

Mount the password file into the Traefik container; it is read at middleware construction, so rotation requires reloading the middleware. Never put credentials in the configuration or repository. TLS verifies the server using system trust; custom CA/client certificate options are not exposed. `timeout` must be positive and no more than 10 seconds. Only standalone Redis is supported; Sentinel, Cluster, pooling, retries and automatic memory fallback are not implemented. Each operation uses a bounded connection. Redis ACLs must permit authentication/database selection where configured and the GET, SET, PTTL, PEXPIRE and EVAL operations used by the adapter.

## HTTP behavior

Only complete 200 GET responses are stored. Authorization, Cookie, Range, Upgrade, client conditional headers, Pragma and request `no-store` bypass caching. Responses with Set-Cookie, Vary, Trailer, private, no-store or must-understand are not stored. Streaming/flush, SSE, gRPC and oversized bodies are forwarded without storage.

Origin `s-maxage`, `max-age`, Expires and Age can shorten configured freshness. `no-cache` requires validation before reuse. Request `no-cache`, max-age and min-fresh are respected for cache reuse. Safe requests with `only-if-cached` return 504 when no usable cached response exists, without contacting the origin. There is no stale serving or full RFC 9111 conformance claim; extensions such as stale-while-revalidate and stale-if-error are not implemented.

`X-Cache-Status` reports HIT, MISS, REVALIDATED, or BYPASS on handled paths. Some direct bypass/streaming paths have no status header. It is diagnostic, not a security assertion.
