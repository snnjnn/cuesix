# smc-apisix-rules

Companion chart for `smc-apisix`. It creates snippet ConfigMaps/Secrets that the
`smc-apisix` watcher sidecar consumes, plus optional Services/Ingresses that
route traffic into the shared APISIX gateway.

## Relationship to smc-apisix

- `smc-apisix` runs APISIX + Sixpack + watcher sidecar.
- This chart publishes snippet resources labeled
  `cuesisx.apisix/snippet: "true"` so the watcher sidecar picks them up and
  triggers a compile.
- Optional Services/Ingresses created here can point at the common service
  created by `smc-apisix`, so traffic flows through APISIX without duplicating
  selectors.

## Usage

Install against the same namespace as `smc-apisix`:

```bash
helm install smc-apisix-rules ./smc-apisix-rules
```

## Snippet format and schema

Snippets are partial APISIX configurations (for example, `routes`, `upstreams`,
`services`, `consumers`, and other supported resources). Refer to the APISIX
Admin API documentation for the full set of resources and shapes.

APISIX resources support an optional `id`; it is strongly recommended to set it
to avoid merge conflicts when snippets are combined

For schema details, enable the optional control API ingress in the
`smc-apisix` chart and query `/v1/schema` to inspect the supported schema
objects for your deployed APISIX version.

TLS certificate Secrets must be labeled (for example,
`cuesisx.apisix/cert: "true"`) because the watcher sidecar filters by labels,
not by Secret type.

## Common service wiring

If a snippet does not define a `service`, its ingress (if any) targets the
service created by `smc-apisix`. Configure that mapping here:

```yaml
commonService:
  name: smc-apisix
  port: 80
  targetPort: http
  selector:
    app.kubernetes.io/name: smc-apisix
    app.kubernetes.io/instance: smc-apisix
```

If a snippet defines a `service`, it still points to the same selector and
targetPort, but with its own `name` and `port`, so you can expose different
frontends without duplicating selectors.

## Examples

The following examples mirror the richer snippets in `smc-apisix-rules/values.yaml`
and explain how names and routing are resolved.

### Example: ConfigMap snippet with a dedicated service

This snippet defines an upstream and route that match the service name and the
fully qualified service DNS name. A per-item Service is created, but it still
selects the `smc-apisix` pods and routes to the common targetPort. The new
Service name is used by the route host match and can also be used for Ingress
hostnames. All APISIX resources can include an optional `id`; it is strongly
recommended to set it to avoid merge conflicts across snippets.

```yaml
configmaps:
  example:
    snippet: |-
      upstreams:
        - id: example_upstream
          nodes:
            "upstream_service:8080": 1
      routes:
        - id: example_proxy
          uri: /*
          # Match the service name
          hosts:
            - example-gated-service
            - example-gated-service.namespace_name.svc.cluster.local
          upstream_id: example_upstream
    service:
      name: example-gated-service
      port: 80
    ingress:
      enabled: false
      className: ""
      annotations: {}
      hosts:
        - host: example.local
          paths:
            - path: /
              pathType: ImplementationSpecific
      tls: []
```

### Example: Secret snippet using the common service

This snippet does not define a per-item `service`, so any Ingress created here
will point at the common service (`commonService.name`). The route hosts should
only match the Ingress host in this case; do not include the common service
name unless you also create a dedicated service. All APISIX resources can
include an optional `id`; it is strongly recommended to set it to avoid merge
conflicts across snippets.

```yaml
secrets:
  secure:
    snippet: |-
      plugin_configs:
      - id: example-ingress-plugins
        limit-req:
          rate: 2
          burst: 5
          key: remote_addr
      routes:
        - id: example-ingress
          uri: /*
          hosts:
            # Match the ingress name and the service name
            - secure.url.com
          plugin_config_id: example-ingress-plugins
          upstream:
            nodes:
              "127.0.0.1:8080": 1
    ingress:
      enabled: true
      className: ""
      annotations: {}
      hosts:
        - host: secure.url.com
          paths:
            - path: /
              pathType: Prefix
      tls: []
```

### Gateway-wide settings

Gateway-wide settings can be provided as a snippet without a `service` or
`ingress`. This example mirrors `apisix/volumes/input/global.yaml`:

```yaml
configmaps:
  global:
    snippet: |-
      global_rules:
        - id: global-rate-limit
          plugins:
            ua-restriction:
              denylist:
                - curl
                - wget
                - python
                - sqlmap
                - nikto
            response-rewrite:
              headers:
                set:
                  X-Frame-Options: DENY
                  X-Content-Type-Options: nosniff
                  Referrer-Policy: no-referrer
                  Permissions-Policy: interest-cohort=()
                remove:
                  - Server
                  - X-Powered-By
                  - X-APISIX-Upstream-Status
            limit-req:
              rate: 5
              burst: 20
              key: remote_addr
            limit-conn:
              conn: 20
              burst: 10
              key: remote_addr
              default_conn_delay: 200
```
