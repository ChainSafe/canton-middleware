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
| `networkpolicy.yaml` | Limits the indexer to the API server, and both metrics ports to Prometheus |
| `config/*.yaml` | Service configuration, generated into ConfigMaps by the kustomization |
| `secrets.example.yaml` | Template for the three Secrets |
| `kustomization.yaml` | Ties the manifests together and pins the image release |

Every pod does the following:
- Runs its database migrations at startup, through the image's entrypoint, and exits if they fail.
  The migrations are idempotent.
- Runs as a non-root user with all capabilities dropped and no service account token.
- Exposes Prometheus metrics on port 9090 at `/metrics`, with `prometheus.io/*` scrape annotations.

## Prerequisites

- **Images.** The `canton-erc20-api` (API server) and `canton-middleware` (relayer) packages on GHCR are
  currently private; `canton-indexer` is public. Until all three are public, build the images yourself
  (see [Building the images](#building-the-images)).
- Kubernetes 1.27 or later, and `kubectl`, which includes kustomize. `networkpolicy.yaml` only takes effect
  if the cluster's CNI enforces NetworkPolicy.
- A Canton participant whose Ledger API is reachable from the cluster:
  - The canton-erc20 packages must be uploaded and vetted. See `deployments/canton-config/deploy-dars.canton`.
  - The issuer party must be allocated.
- OAuth2 client-credentials clients for the participant's Ledger API, one per service. Each client's
  token `sub` is its Ledger API user ID. Create that user on the participant and grant these rights:

  | Service | Ledger API rights | Why |
  |---|---|---|
  | API server | `ParticipantAdmin`, and `CanActAs` on the issuer party | Allocates a party for each registered user and grants itself `CanActAs` on it; acts and reads as the issuer |
  | Indexer | `CanReadAsAnyParty` | Streams every party's updates |
  | Relayer | `CanActAs` on the issuer party | Mints and burns bridged tokens as the issuer |

  A missing right shows up as `PERMISSION_DENIED` from the Ledger API.
- PostgreSQL with three databases: `erc20_api`, `canton_indexer` and `relayer`.
- For the relayer:
  - an Ethereum RPC endpoint (HTTP and WebSocket)
  - the deployed `CantonBridge` and token contracts
  - the key of the account that holds the relayer role on the bridge

## Building the images

Build the images from the release tag the same way CI does, then point the kustomization at them. The
context is the repository root:

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

If your cluster needs credentials for the registry, add an `imagePullSecrets` entry to each Deployment. To
pin an exact build rather than a tag, add `digest: sha256:…` to each entry.

## Deploy

1. Fill in every value marked `CHANGE ME` in `config/api-server.yaml`, `config/indexer.yaml`,
   `config/relayer.yaml`, `ingress.yaml` and `networkpolicy.yaml`. To serve USDCx or another token issued by
   a different party, uncomment the USDCx entries in `config/api-server.yaml`.
2. Create the namespace and the Secrets:

   ```sh
   kubectl apply -f deployments/kubernetes/namespace.yaml
   cp deployments/kubernetes/secrets.example.yaml deployments/kubernetes/secrets.yaml
   # edit secrets.yaml, then:
   kubectl apply -f deployments/kubernetes/secrets.yaml
   ```

   `secrets.yaml` is git-ignored. Each key becomes an environment variable that fills a `${VAR}` placeholder
   in the config files. The template shows how to generate `JWT_PRIVATE_KEY` and `CANTON_MASTER_KEY`, and
   which characters need encoding. Store `CANTON_MASTER_KEY` somewhere durable: it encrypts custodial user
   keys at rest, and losing it makes those keys unrecoverable.
3. Apply the manifests:

   ```sh
   kubectl apply -k deployments/kubernetes
   ```

4. Wait for the rollouts:

   ```sh
   kubectl -n canton-middleware rollout status deployment/canton-indexer
   kubectl -n canton-middleware rollout status deployment/canton-middleware-api
   kubectl -n canton-middleware rollout status deployment/canton-bridge-relayer
   ```

   A finished rollout only shows that the processes started and their migrations ran: `/health` doesn't
   check the ledger. The relayer's readiness probe (`/ready`) is the only one that reflects its connections.
5. Check that the indexer is reading the ledger. Its log shouldn't repeat `canton stream disconnected`, and
   once any token exists on the participant, the indexer lists it:

   ```sh
   kubectl -n canton-middleware logs deployment/canton-indexer --since=5m | grep -c "canton stream disconnected"
   kubectl -n canton-middleware port-forward svc/canton-indexer 8082:8082 &
   curl -s localhost:8082/indexer/v1/admin/tokens
   ```

6. Allowlist your users. With `skip_whitelist_check: false`, registration and every
   `eth_sendRawTransaction` are refused for addresses that aren't on the whitelist:

   ```sh
   kubectl -n canton-middleware port-forward svc/canton-middleware-api 8080:8080 &
   curl -s -X POST localhost:8080/admin/whitelist -H "Authorization: Bearer $ADMIN_API_KEY" \
     -H 'Content-Type: application/json' -d '{"evm_address":"0x…","note":"first user"}'
   ```

7. Register an allowlisted address through the dApp. Registration allocates a Canton party, so it is the
   first step that proves the API server can reach the ledger with the right permissions. Then check the
   public endpoint:

   ```sh
   curl -s https://middleware.example.com/eth -H 'Content-Type: application/json' \
     -d '{"jsonrpc":"2.0","id":1,"method":"eth_chainId","params":[]}'
   ```

   `eth_chainId` returns the `eth_rpc.chain_id` from `config/api-server.yaml`, in hex.

## Upgrades

- **New release:** set `newTag` for all three images in `kustomization.yaml` to the new release, keeping them
  on the same version. Check the release notes for config changes, then apply again with
  `kubectl apply -k`. Each service applies any new migrations when it starts.
- **Config changes:** ConfigMap names carry a hash of their content, so editing a config file and applying
  again rolls the Deployment that mounts it.

## Notes

- **One replica each.** The manifests run one replica of each service. The indexer and relayer use the
  `Recreate` strategy, so two copies never process the same ledger or Ethereum events at once.
- **Unauthenticated internal APIs.** The indexer's `/indexer/v1/admin` routes return balances, transfers and
  events for every party, and the relayer serves its transfer list without authentication.
  `networkpolicy.yaml` keeps them away from other pods. Don't expose either service outside the cluster.
- **`/admin` on the Ingress.** The Ingress routes every path to the API server, including
  `/admin/whitelist`. It requires `ADMIN_API_KEY`, but you can also block `/admin` at your ingress
  controller or WAF and manage the whitelist through `kubectl port-forward` as in step 6.
- **Visibility.** The indexer sees only what this participant's parties can see, so each operator runs its
  own indexer against its own participant.
- **Resources.** Requests and limits follow ChainSafe's production API server (100m CPU and 128Mi requested,
  1 CPU and 1Gi limit). Adjust them to your load.
- **Config tested in CI.** `pkg/config/kubernetes_test.go` loads the files in `config/` with the services'
  strict config loaders, with and without the USDCx entries, so a config key that no longer exists fails CI.
