# Releases

The initial version is `v0.1.0`. `.github/workflows/ci-release.yml` validates main pushes, pull requests and manual runs. Only version-tag push events publish packages and releases; manual workflow dispatch does not publish.

The validation job runs formatting checks, race tests, vet, and Compose integration against real Traefik and Redis. Publishing depends on that job. The tag must be `vMAJOR.MINOR.PATCH` and match the `Version` constant in `http_cache.go`.

The publishing job uses the repository's short-lived GITHUB_TOKEN with `packages: write` and `contents: write`; no personal token is required. It publishes the Linux AMD64 bundled Traefik image to `ghcr.io/modulairy/traefik-http-cache:<tag>`, with provenance and SBOM, then creates a GitHub Release identifying the image digest. There is no moving `latest` tag.

## Repository setup

Enable GitHub Actions and allow the pinned actions in organization policy. Allow this repository to publish the matching GHCR package; existing packages may require explicitly granting repository Actions access. Package visibility is separate from repository visibility. Change package visibility to public if unauthenticated image pulls are intended. Private pulls require authorized credentials/imagePullSecrets. These are account settings, not effects of committing the workflow.

## Release checklist

1. Update the source version and changelog and review the changes. Preserve the root plugin entry point and `.traefik.yml` layout from the [official Traefik example](https://github.com/traefik/plugindemo), and include the `internal/store` supporting package in the container build.
2. Run native checks and the Compose suite successfully.
3. Push the reviewed source commit, then its version tag.
4. Confirm the validation and publish jobs succeed, inspect the release and GHCR package, and pull the exact tag/digest.

Example maintainer commands for the initial release:

```sh
git tag -a v0.1.0 -m 'Release v0.1.0'
git push origin v0.1.0
```

Do not move a published tag. Correct defects in a new version. Action commit pins were resolved from the upstream repositories; updating them requires review. The container build uses the Traefik version pinned in `container/Dockerfile`, and Compose pins its Go and Redis versions.

Release distributions must retain the project's Apache 2.0 `LICENSE` and `NOTICE`, along with applicable third-party notices. `THIRD_PARTY_NOTICES.md` identifies bundled Traefik and Go notices; it does not replace the licenses of base-image packages or transitive dependencies. Refresh component notices when updating upstream versions.
