# Releases

The initial version is `v0.1.0`. `.github/workflows/ci-release.yml` validates main pushes, pull requests and manual runs. Only version-tag push events publish packages and releases; manual workflow dispatch does not publish.

The separate `.github/workflows/image-tag-alias.yml` migration is an exception: its scoped push trigger or manual retry adds the suffixed alias to the existing immutable image index digest and updates the existing release notes. It does not rebuild the image, change the source Git tag, or overwrite the legacy image. Reusing the index preserves its attestations.

The validation matrix runs formatting checks, race tests, vet, and Compose integration against real Traefik and Redis on native AMD64 and ARM64 runners. Publishing depends on that job. The tag must be `vMAJOR.MINOR.PATCH` and match the `Version` constant in `http_cache.go`.

The publishing job uses the repository's short-lived GITHUB_TOKEN with `packages: write`; the separate release job uses `contents: write`. No personal token is required. It publishes the Linux AMD64/ARM64 bundled Traefik image to `ghcr.io/modulairy/traefik-http-cache:<plugin-tag>-traefik-v<major.minor>`, with provenance and SBOM, then tests the published digest on both native architectures before creating a GitHub Release identifying the image tag, exact Traefik version and digest. A pushed package alone is not proof that published-image verification has passed. There is no moving `latest` tag.

The workflow derives the exact Traefik version from the digest-pinned `ARG TRAEFIK_VERSION` in `container/Dockerfile`. For plugin `v0.1.0` and base `v3.7.16`, the image tag is `v0.1.0-traefik-v3.7` ; future builds also set OCI label `io.traefik.version` to the full base version. The initial alias does not add a label to the immutable original image; its exact version is verified at runtime and recorded in release notes. Only the container tag gains the suffix: source Git tags and remote plugin registration use the unsuffixed plugin version for Go module/catalog resolution. Future publishing uses the suffixed format only. The already published legacy `v0.1.0` image is preserved unchanged; adding an alias to its existing digest is a separate migration, not a rebuild or a source-tag change.

## Repository setup

Enable GitHub Actions and allow the pinned actions in organization policy. Allow this repository to publish the matching GHCR package; existing packages may require explicitly granting repository Actions access. Package visibility is separate from repository visibility. Change package visibility to public if unauthenticated image pulls are intended. Private pulls require authorized credentials/imagePullSecrets. These are account settings, not effects of committing the workflow.

## Plugin catalog

**Availability check — 2026-10-26:** [HTTP Cache by Modulairy](https://plugins.traefik.io/plugins/6adf0931bda0e3c02a2759e0/http-cache) is listed with current version `v0.1.1`. Its download and archive-hash validation endpoints both returned HTTP 200. A clean Traefik 3.7.16 container also downloaded v0.1.1 through remote mode, logged Plugins loaded, and returned HTTP 200 from a middleware-attached test route. The prior 2026-10-25 discovery failure is resolved. Verify startup on your intended Traefik version before deployment. GHCR provides an optional bundled local-mode image and is separate from catalog discovery.

Before expecting catalog discovery, verify:

- The GitHub repository is public, is not a fork, and has the `traefik-plugin` topic.
- Root `.traefik.yml` contains a valid manifest and usable `testData`.
- Root `go.mod` identifies the module, and the intended source version has a Git tag.
- External package dependencies, if introduced, are vendored and committed. The current plugin has none; `internal/store` is part of this module.

The catalog discovers eligible repositories daily. If its importer opens an error issue, fix the reported problem and close the issue so discovery can retry. A pushed tag is not confirmation of listing: verify the catalog entry and test a fresh Traefik remote startup separately. These requirements and timing follow the [official packaging checklist](https://plugins.traefik.io/create).

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

## v0.1.1

Adds linux/arm64 alongside linux/amd64 without changing cache behavior or the pinned Traefik 3.7.16 base. Image tag: `v0.1.1-traefik-v3.7`. Each release uses an OCI index containing both runtime architectures plus SBOM/provenance attestations. Older v0.1.0 tags and digests remain unchanged. Native runners validate both architectures; published-image tests use the exact digest, not a rebuilt image.

The initial v0.1.1 run published both images but its final test command used an unsupported Compose option. The dedicated `verify-v0.1.1.yml` recovery workflow verifies the same immutable published digest on both native architectures and creates the release only after success. It does not rebuild or move the source/image tags. Future releases use the corrected main workflow.

## Catalog presentation

The root `.traefik.yml` defines the catalog name, summary, icon and sharing banner. README content supplies the detail page; `bannerPath` does not insert a README header. Keep module identity and the `httpCache` alias stable. Use absolute documentation links where catalog rendering would otherwise resolve paths against plugins.traefik.io.

Catalog artwork and metadata changes should be included in the next reviewed source version. Do not move `v0.1.1` or earlier tags. A branding-only PR does not itself publish a release or update the catalog. After release and discovery, verify the icon, banner, version and remote loading on the actual catalog page. Corporate OCI labels in the release workflow apply only to future image builds.

## v0.1.2

Updates catalog artwork, corporate metadata and documentation. Cache policy and adapters are unchanged; Traefik remains 3.7.16. Source version: 0.1.2. Container tag: 0.1.2-traefik-v3.7, targeting Linux AMD64 and ARM64. Publication is complete only after the tag workflow and both published-image checks succeed.
