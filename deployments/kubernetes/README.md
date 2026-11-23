# Kubernetes deployment

Manifests for running the API server, indexer and bridge relayer on Kubernetes. They describe the same
workloads ChainSafe runs on its own clusters, with environment-specific values replaced by placeholders.
For a single-host setup, use the Docker Compose files in the repository root instead.

## Contents

| File | Contents |
|---|---|
| `api-server.yaml` | ServiceAccount, Service and Deployment for the API server (JSON-RPC at `/eth`, REST API) |
| `indexer.yaml` | ServiceAccount, Service and Deployment for the indexer |
| `relayer.yaml` | ServiceAccount, Service and Deployment for the bridge relayer |
| `ingress.yaml` | Ingress for the API server. The indexer and relayer stay internal |
| `config/*.yaml` | Service configuration, generated into ConfigMaps by the kustomization |
| `secrets.example.yaml` | Template for the three Secrets |
| `kustomization.yaml` | Ties the manifests together and pins the image release |

Every pod does the following:
- Runs its database migrations in an init container before the service starts. The migrations are
  idempotent.
- Runs as a non-root user with all capabilities dropped and no service account token.
- Exposes Prometheus metrics on port 9090 at `/metrics`, with `prometheus.io/*` scrape annotations.

## Prerequisites

- Kubernetes 1.27 or later, and `kubectl`, which includes kustomize.
- A Canton participant whose Ledger API is reachable from the cluster:
  - The canton-erc20 packages must be uploaded and vetted. See `deployments/canton-config/deploy-dars.canton`.
  - The issuer party must be allocated.
- OAuth2 client-credentials clients for the participant's Ledger API, one per service.
- PostgreSQL with three databases: `erc20_api`, `canton_indexer` and `relayer`.
- For the relayer:
  - an Ethereum RPC endpoint (HTTP and WebSocket)
  - the deployed `CantonBridge` and token contracts
  - the key of the account that holds the relayer role on the bridge
- Pull access to the images at `ghcr.io/chainsafe`, or your own builds of them (see
  [Building the images](#building-the-images)). If your cluster needs credentials for the registry, add an
  `imagePullSecrets` entry to each Deployment.

## Deploy

1. Fill in every value marked `CHANGE ME` in `config/api-server.yaml`, `config/indexer.yaml`,
   `config/relayer.yaml` and `ingress.yaml`.
2. Create the namespace and the Secrets:

   ```sh
   kubectl apply -f deployments/kubernetes/namespace.yaml
   cp deployments/kubernetes/secrets.example.yaml deployments/kubernetes/secrets.yaml
   # edit secrets.yaml, then:
   kubectl apply -f deployments/kubernetes/secrets.yaml
   ```

   `secrets.yaml` is git-ignored. Each key becomes an environment variable that fills a `${VAR}` placeholder
   in the config files. `CANTON_MASTER_KEY` encrypts custodial user keys at rest: store it somewhere durable,
   because losing it makes those keys unrecoverable.
3. Apply the manifests:

   ```sh
   kubectl apply -k deployments/kubernetes
   ```

4. Check that the services are up:

   ```sh
   kubectl -n canton-middleware rollout status deployment/canton-indexer
   kubectl -n canton-middleware rollout status deployment/canton-middleware-api
   kubectl -n canton-middleware rollout status deployment/canton-bridge-relayer
   curl -s https://middleware.example.com/health
   curl -s https://middleware.example.com/eth -H 'Content-Type: application/json' \
     -d '{"jsonrpc":"2.0","id":1,"method":"eth_chainId","params":[]}'
   ```

   `eth_chainId` returns the `eth_rpc.chain_id` from `config/api-server.yaml`, in hex.

## Building the images

To run images from your own registry, build them from the release tag the same way CI does, then point the
kustomization at them. The context is the repository root:

| Image in `kustomization.yaml` | Service | Dockerfile |
|---|---|---|
| `ghcr.io/chainsafe/canton-erc20-api` | API server | `cmd/api-server/Dockerfile` |
| `ghcr.io/chainsafe/canton-indexer` | Indexer | `cmd/indexer/Dockerfile` |
| `ghcr.io/chainsafe/canton-middleware` | Bridge relayer | `cmd/relayer/Dockerfile` |

```sh
git checkout v0.10.0
REGISTRY=registry.example.com/canton-middleware
# VERSION is what the API server reports in web3_clientVersion.
docker build -f cmd/api-server/Dockerfile --build-arg VERSION="v0.10.0-$(git rev-parse HEAD)" \
  -t "$REGISTRY/api-server:v0.10.0" .
docker build -f cmd/indexer/Dockerfile -t "$REGISTRY/indexer:v0.10.0" .
docker build -f cmd/relayer/Dockerfile -t "$REGISTRY/relayer:v0.10.0" .
docker push "$REGISTRY/api-server:v0.10.0"
docker push "$REGISTRY/indexer:v0.10.0"
docker push "$REGISTRY/relayer:v0.10.0"
```

Add `--platform linux/amd64` when building on an ARM machine for an amd64 cluster. Then replace the `images`
block in `kustomization.yaml`:

```yaml
images:
  - name: ghcr.io/chainsafe/canton-erc20-api
    newName: registry.example.com/canton-middleware/api-server
    newTag: v0.10.0
  - name: ghcr.io/chainsafe/canton-indexer
    newName: registry.example.com/canton-middleware/indexer
    newTag: v0.10.0
  - name: ghcr.io/chainsafe/canton-middleware
    newName: registry.example.com/canton-middleware/relayer
    newTag: v0.10.0
```

## Upgrades

- **New release:** set `newTag` for all three images in `kustomization.yaml` to the new release, keeping them
  on the same version. Check the release notes for config changes, then apply again with
  `kubectl apply -k`. The init containers apply any new migrations before the services start.
- **Config changes:** ConfigMap names carry a hash of their content, so editing a config file and applying
  again rolls the Deployment that mounts it.

## Notes

- **One replica each.** The manifests run one replica of each service. The indexer and relayer use the
  `Recreate` strategy, so two copies never process the same ledger or Ethereum events at once.
- **Visibility.** The indexer sees only what this participant's parties can see, so each operator runs its
  own indexer against its own participant.
- **Config tested in CI.** `pkg/config/kubernetes_test.go` loads the files in `config/` with the services'
  strict config loaders, so a config key that no longer exists fails CI.
