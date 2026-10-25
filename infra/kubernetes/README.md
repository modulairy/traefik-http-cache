# Kubernetes integration examples

These manifests assume an existing `apps` namespace, `catalog` Service on port 8080, and a configured Traefik controller. Names and hosts are examples. Choose one route example, rather than installing all three for the same host.

Use `ghcr.io/modulairy/traefik-http-cache:v0.1.0` after its release workflow succeeds. The image includes the local plugin source. Add this fragment to the controller's existing static configuration and restart it:

```yaml
experimental:
  localPlugins:
    httpCache:
      moduleName: github.com/modulairy/traefik-http-cache
```

Enable the Kubernetes CRD provider and install matching Traefik CRDs/RBAC. Also enable the Kubernetes Ingress provider for Ingress, or the Kubernetes Gateway provider and Gateway API CRDs/RBAC for HTTPRoute. The Gateway API example assumes an existing `traefik-gateway` Gateway in `apps` with a listener allowing the route. Middleware is attached to HTTPRoute, not directly to Gateway.

Apply `middleware.yaml` and the selected `ingress.yaml`, `ingressroute.yaml`, or `httproute.yaml` using your explicitly selected cluster/context. No controller, Gateway, backend, Redis or credentials are provisioned by these examples. TLS, resource limits, network policy and deployment credentials belong to the existing cluster configuration.

For Redis, replace `spec.plugin.httpCache.store` with the Redis configuration in `docs/configuration.md`, mount the password Secret as a file into Traefik, and allow network access to the selected Redis endpoint. In-memory caches and invalidation are independent per middleware instance/pod. Use Redis for shared cache state.

Check controller events/logs and route accepted/resolved-reference status, then confirm MISS/HIT and revalidation against a public test endpoint. These manifests have not been applied to a cluster by the local Compose tests.
