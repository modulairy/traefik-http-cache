# Kubernetes integration examples

These manifests assume an existing `apps` namespace, `catalog` Service on port 8080, and a configured Traefik controller. Names and hosts are examples. Choose one route example, rather than installing all three for the same host.

Remote loading was verified with `v0.1.1`, which is listed in the [Plugin Catalog](https://plugins.traefik.io/plugins/6adf0931bda0e3c02a2759e0/http-cache). The examples target `v0.1.2`; confirm that release is available through the catalog before using them. Add this fragment to the existing controller's static configuration and restart it; no custom image or plugin-copying init container is required. Verify loading before routing production traffic:

```yaml
experimental:
  plugins:
    httpCache:
      moduleName: github.com/modulairy/traefik-http-cache
      version: v0.1.2
```

Traefik downloads and loads the plugin at startup, inside the controller process. Permit access to its plugin distribution endpoints and check startup logs for successful loading. Version changes require a restart. Remote availability/catalog discovery must be verified independently; the local Compose suite does not validate them. See the [official installation guide](https://plugins.traefik.io/install) and [release checklist](../../docs/releases.md#plugin-catalog).

For offline plugin loading, use the optional bundled image `ghcr.io/modulairy/traefik-http-cache:v0.1.2-traefik-v3.7` after its release workflow passes. This release supports Linux AMD64 and ARM64; pin the released index digest for reproducible deployments. The suffix identifies the Traefik minor line; future builds record the exact bundled version in label `io.traefik.version`. The initial alias retains the original metadata; consult its release notes for the verified exact version. The legacy image tag `v0.1.0` remains unchanged. In local mode replace `plugins` with `localPlugins` and omit `version`, as in `../traefik.yml`. Remote plugin configuration above keeps `version: v0.1.2` without the container suffix. Choose one registration mode for the `httpCache` alias. Dynamic Middleware manifests are the same in either mode.

Enable the Kubernetes CRD provider and install matching Traefik CRDs/RBAC. Also enable the Kubernetes Ingress provider for Ingress, or the Kubernetes Gateway provider and Gateway API CRDs/RBAC for HTTPRoute. The Gateway API example assumes an existing `traefik-gateway` Gateway in `apps` with a listener allowing the route. Middleware is attached to HTTPRoute, not directly to Gateway.

Apply `middleware.yaml` and the selected `ingress.yaml`, `ingressroute.yaml`, or `httproute.yaml` using your explicitly selected cluster/context. No controller, Gateway, backend, Redis or credentials are provisioned by these examples. TLS, resource limits, network policy and deployment credentials belong to the existing cluster configuration.

For Redis, replace `spec.plugin.httpCache.store` with the Redis configuration in `docs/configuration.md`, mount the password Secret as a file into Traefik, and allow network access to the selected Redis endpoint. In-memory caches and invalidation are independent per middleware instance/pod. Use Redis for shared cache state.

Check controller events/logs and route accepted/resolved-reference status, then confirm MISS/HIT and revalidation against a public test endpoint. These manifests have not been applied to a cluster by the local Compose tests.
