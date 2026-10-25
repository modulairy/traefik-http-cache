# Changelog

Original project code is licensed under Apache-2.0. Upstream notices are retained in `licenses/` and included in container images.

## v0.1.0

Initial implementation:

- Traefik local middleware plugin with rule-based public response caching.
- Independent bounded memory and standalone Redis adapters.
- Path, header and Content-Type conditions.
- HTTP cache directives, conditional revalidation and generation-based same-URI invalidation.
- Local Compose environment, Kubernetes examples, tests and tag-driven container/release workflow.

See `docs/design.md` for deliberate limitations. Availability of the release image depends on a successful publishing workflow.
