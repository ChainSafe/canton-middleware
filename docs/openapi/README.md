# OpenAPI specifications

OpenAPI 3.1 descriptions of the HTTP APIs, one per service.

| File | Service | Default port (docker-compose) |
|---|---|---|
| [`api-server.yaml`](api-server.yaml) | API server: REST API and the `/eth` JSON-RPC endpoint | 8081 |
| [`indexer.yaml`](indexer.yaml) | Indexer private read API | 8082 |
| [`relayer.yaml`](relayer.yaml) | Relayer status API | 8080 |

Some API server routes are mounted only when configured:

| Routes | Mounted when |
|---|---|
| `/auth/*` and `/.well-known/jwks.json` | The `auth` block is set |
| `/admin/*` | `admin.enabled` is set |
| `/eth` | `eth_rpc.enabled` is set |

The authentication model is described in [`../SECURITY_AND_PRIVACY_MODEL.md`](../SECURITY_AND_PRIVACY_MODEL.md).

## Lint

```bash
npx @redocly/cli lint --config docs/openapi/redocly.yaml \
  docs/openapi/api-server.yaml docs/openapi/indexer.yaml docs/openapi/relayer.yaml
```

## Preview

```bash
npx @redocly/cli preview-docs docs/openapi/api-server.yaml
```

## Keeping the specs current

Routes are registered in these handler files:

- `pkg/user/service/http.go`
- `pkg/transfer/http.go`
- `pkg/auth/service/http.go`
- `pkg/token/http.go`
- `pkg/user/whitelist/http.go`
- `pkg/registry/handler.go`
- `pkg/ethrpc/service/http.go`
- `pkg/indexer/service/http.go`
- `pkg/relayer/service/http.go`

Update the matching spec in the same change as any route or payload edit.
