# Changelog

Original project code is licensed under Apache-2.0. Upstream notices are retained in `licenses/` and included in container images.

## v0.1.3

- Opt-in per-rule `varyHeaders: [Accept-Encoding]` support, with distinct raw header keys for gzip and identity representations.
- Preserve compressed response bytes and validators; reject unsupported/wildcard Vary and refetch when 304 metadata changes representation selection or encoding.
- Native memory/Redis and actual Traefik/Yaegi compressed-asset regression coverage. Existing privacy, invalidation and capacity protections remain in place.

## v0.1.2

- Modulairy catalog icon, sharing banner and README presentation.
- Corporate support links and OCI image metadata.
- Updated remote installation guidance following successful catalog publication.
- No cache policy or adapter behavior changes; Traefik remains pinned to 3.7.16.

## v0.1.0

Initial implementation:

- Traefik local middleware plugin with rule-based public response caching.
- Independent bounded memory and standalone Redis adapters.
- Path, header and Content-Type conditions.
- HTTP cache directives, conditional revalidation and generation-based same-URI invalidation.
- Local Compose environment, Kubernetes examples, tests and tag-driven container/release workflow.

See `docs/design.md` for deliberate limitations. Availability of the release image depends on a successful publishing workflow.
