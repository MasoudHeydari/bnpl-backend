# Local Envoy configuration

`config.yaml` is the **development-only snapshot** of the full platform Envoy
configuration (BNPL, Identity and Merchant routing). It is stored here for
developers running BNPL with `docker-compose.local.yml`.

Before starting the Envoy container:

1. Set the listener and upstream IP addresses to match your host/network.
2. Ensure the TLS certificate paths under `/etc/envoy/certs` match the actual
   certificate volume mounted in the Compose service.
3. Verify the virtual host for short-link resolution matches the configured
   `SHORT_LINK_PUBLIC_HOST`. The snapshot uses `dpay.ir` and its
   `443`/`8443` authority variants.
4. Validate the private configuration using the container's Envoy binary:
   `envoy --mode validate -c /etc/envoy/envoy.yaml`.

This is an **entire-platform gateway snapshot**, not a BNPL-only route
fragment. The authoritative renderer remains in the external
`platform-infra` repository. Do not merge changes to unrelated upstreams
into this snapshot without coordinating with their owners.

Lua body diagnostics are for **isolated local development only**:

- Both request and response JSON use Envoy's \`body()\` API and are recorded
  under \`request_body\`/\`response_body\` when the actual payload is at most
  **4096 bytes**. If \`Content-Length\` declares a larger body, it is skipped
  before buffering; chunked JSON without Content-Length is supported but may
  need to buffer the full payload before the size check.
- Known sensitive API families (Identity/auth, hosted checkout, payment,
  partner, merchant signup/onboarding) are recorded as \`[REDACTED]\` without
  reading the body. This path list is not a complete privacy boundary:
  **other JSON responses can still contain sensitive data**.
- Inspect \`request_body_capture_state\` and \`response_body_capture_state\`
  for \`captured\`, \`redacted_sensitive_path\`, \`skipped_non_json\`,
  \`skipped_large\`, \`skipped_compressed\`, \`read_failed\`, and other
  diagnostic outcomes. \`BYTES_RECEIVED\` and \`BYTES_SENT\` measure traffic,
  not successful body capture.

Do not ship this raw-body logging configuration to Production. Production
logs must use trusted metadata or an explicit, audited field allowlist.

For certificate generation and `/etc/hosts` setup, see
[`doc/local-development/mkcert/README.md`](../../../../doc/local-development/mkcert/README.md).
