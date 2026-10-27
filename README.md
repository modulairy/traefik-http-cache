![Modulairy HTTP Cache — rule-based caching for Traefik, with Redis and memory stores](https://raw.githubusercontent.com/modulairy/traefik-http-cache/main/.assets/banner.png)

# HTTP Cache for Traefik

Cache public HTTP responses with precise rules and a choice of Redis or in-memory storage. Built by [Modulairy](https://modulairy.com/) to make infrastructure simpler to operate.

[Plugin catalog](https://plugins.traefik.io/plugins/6adf0931bda0e3c02a2759e0/http-cache) · [Documentation](https://github.com/modulairy/traefik-http-cache/blob/main/docs/configuration.md) · [Releases](https://github.com/modulairy/traefik-http-cache/releases) · [Report an issue](https://github.com/modulairy/traefik-http-cache/issues)

## Cache what matters

| Capability | What it gives you |
| --- | --- |
| Redis or memory | Shared cache storage across instances, or bounded local storage. |
| Precise matching | Exact paths, prefixes, contains and regex; request and response header conditions. |
| Public responses only | Conservative handling of cookies, authorization and HTTP cache directives. |
| Conditional revalidation | ETag and Last-Modified checks when a response expires, or on every use. |
| Encoded public assets | Opt-in `Vary: Accept-Encoding` support keeps gzip and identity CSS/JS variants separate. |
| Kubernetes integration | Middleware examples for Ingress, IngressRoute and Gateway API HTTPRoute. |

The plugin runs inside Traefik through Yaegi. Redis is optional; memory mode needs no external cache service.

## Start with a public route

Add the published plugin to Traefik's **static configuration**, then restart Traefik:

```yaml
experimental:
  plugins:
    httpCache:
      moduleName: github.com/modulairy/traefik-http-cache
      version: v0.1.3
```

Define a middleware in the **dynamic configuration** and attach it to your public route:

```yaml
http:
  middlewares:
    public-cache:
      plugin:
        httpCache:
          keyPrefix: my-app-public-v1
          store:
            type: memory
            memory:
              maxEntries: 10000
              maxBytes: 67108864
          rules:
            - name: assets
              request:
                path:
                  type: prefix
                  value: /assets/
              cache:
                ttl: 1h
                retention: 24h
```

Only explicitly public, non-personalized responses should be cached. Use a unique `keyPrefix` per application/security boundary. Backend response headers still determine whether a response can be stored; matching a path does not override privacy protections.

Remote mode downloads source during startup. Confirm successful plugin loading before routing traffic to it. See the [installation requirements](https://github.com/modulairy/traefik-http-cache/blob/main/docs/configuration.md) for writable storage, network access and Redis credentials.

## Choose your integration

- **Redis:** [connection, TLS and password-file configuration](https://github.com/modulairy/traefik-http-cache/blob/main/docs/configuration.md).
- **Kubernetes:** [Ingress, IngressRoute and HTTPRoute examples](https://github.com/modulairy/traefik-http-cache/blob/main/infra/kubernetes/README.md).
- **Local development or offline loading:** [bundled Traefik image and Compose stack](https://github.com/modulairy/traefik-http-cache/blob/main/docs/development.md).
- **Behavior and limits:** [cache identity, invalidation and adapter design](https://github.com/modulairy/traefik-http-cache/blob/main/docs/design.md).

The optional bundled image supports Linux AMD64 and ARM64: `ghcr.io/modulairy/traefik-http-cache:v0.1.3-traefik-v3.7`. Remote plugin versions use `v0.1.3`; the Traefik suffix belongs only to container image tags.

## Built by Modulairy

Modulairy connects business software and simplifies daily work, with services across ERP, e-commerce, automation and AI.

[Website](https://modulairy.com/) · [Business enquiries](https://modulairy.com/contact.html) · [GitHub](https://github.com/modulairy)

For plugin bugs and feature requests, use [repository issues](https://github.com/modulairy/traefik-http-cache/issues). Include the plugin/Traefik versions and a minimal configuration with credentials removed.

Original project code is licensed under [Apache 2.0](https://github.com/modulairy/traefik-http-cache/blob/main/LICENSE). Bundled components retain their own licenses; see [third-party notices](https://github.com/modulairy/traefik-http-cache/blob/main/THIRD_PARTY_NOTICES.md). See [asset provenance](https://github.com/modulairy/traefik-http-cache/blob/main/.assets/README.md) for brand artwork.
