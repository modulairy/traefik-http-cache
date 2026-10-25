# Third-party notices

The Apache-2.0 license in this repository applies to original Traefik HTTP Cache code. It does not relicense Traefik, the Go runtime, container base-image packages, or other upstream components. Their copyright notices and license conditions remain applicable.

## Traefik

The release container is built on Traefik **v3.7.16** and contains the upstream Traefik executable. Traefik is distributed under the MIT License. Its full upstream license, including copyright and disclaimer, is reproduced unchanged in [licenses/traefik-LICENSE](licenses/traefik-LICENSE).

Source: [Traefik v3.7.16 LICENSE.md](https://github.com/traefik/traefik/blob/v3.7.16/LICENSE.md).

## Go

The local integration backend is compiled using Go **1.27.3**. Its executable includes Go runtime and standard-library code. The full upstream Go BSD-style license is reproduced unchanged in [licenses/go-LICENSE](licenses/go-LICENSE). The test-backend image is a local test fixture and is not the GHCR release image produced by this project's workflow. Traefik's Go executable also contains runtime and dependency code governed by their respective upstream notices.

Source: [Go 1.27.3 LICENSE](https://github.com/golang/go/blob/go1.27.3/LICENSE).

## Other distribution components

The plugin's `go.mod` has no external module dependencies. This does not mean the bundled Traefik executable or its base image has no dependencies. Retain their upstream notices and inspect the published image's SBOM and upstream distribution metadata for the component inventory and applicable licenses. The two reproduced license files above are not an exhaustive license inventory for the complete container image.

Redis and the Go toolchain images used by Compose are separate upstream images pulled for integration tests; they are not copied into the project's published Traefik image. Those images and their components retain their own licenses. Consult their upstream distribution terms when redistributing them.

The official [Traefik plugin example](https://github.com/traefik/plugindemo) was consulted for repository layout and plugin API conventions. The project does not include its demonstration handler implementation.
