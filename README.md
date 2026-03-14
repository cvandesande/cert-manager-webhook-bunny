# cert-manager-webhook-bunny

Bunny DNS tooling for automated TLS certificate management via ACME DNS-01 challenges. This project provides two ways to use [Bunny DNS](https://bunny.net/dns/) with ACME:

| Tool | Use case |
|------|----------|
| **cert-manager webhook** | Kubernetes clusters running [cert-manager](https://cert-manager.io) |
| **`bunny-certbot-hook`** | Bare-metal / standalone machines running [certbot](https://certbot.eff.org/) |

Both share the same core DNS logic and are published with every release.

This is a maintained fork of the abandoned [gitlab.com/digilol/cert-manager-webhook-bunny](https://gitlab.com/digilol/cert-manager-webhook-bunny) project, updated for:
- Go 1.26
- cert-manager v1.20
- Kubernetes 1.29–1.35
- Installable via Helm chart
- Multi-platform image and binaries (linux/amd64 + linux/arm64)
- Distroless runtime (`gcr.io/distroless/static-debian13:nonroot`, UID 65532) — no shell, minimal attack surface

## Prerequisites

- [cert-manager](https://cert-manager.io/docs/installation/) v1.14+ installed in your cluster
- A [Bunny.net](https://bunny.net) account with DNS zones managed by Bunny DNS
- Helm 3+

## Installation

### 1. Install the webhook via Helm

#### From the OCI registry (recommended)

Releases are published automatically to the GitHub Container Registry as OCI artifacts.

| Artifact | OCI reference |
|----------|---------------|
| Helm chart | `oci://ghcr.io/cvandesande/charts/cert-manager-webhook-bunny` |
| Container image | `ghcr.io/cvandesande/cert-manager-webhook-bunny` |

**Install a specific version:**
```bash
helm install cert-manager-webhook-bunny \
  oci://ghcr.io/cvandesande/charts/cert-manager-webhook-bunny \
  --namespace cert-manager \
  --create-namespace \
  --version 1.1.2
```

**Upgrade to a newer version:**
```bash
helm upgrade cert-manager-webhook-bunny \
  oci://ghcr.io/cvandesande/charts/cert-manager-webhook-bunny \
  --namespace cert-manager \
  --version 1.1.2
```

**List available chart versions** (requires Helm 3.8+, which supports OCI natively):
```bash
# pull the latest and inspect
helm show chart oci://ghcr.io/cvandesande/charts/cert-manager-webhook-bunny --version 1.1.2
```

All releases and their changelogs are listed on the
[GitHub Releases page](https://github.com/cvandesande/cert-manager-webhook-bunny/releases).

#### From source (after cloning)

```bash
git clone https://github.com/cvandesande/cert-manager-webhook-bunny.git
cd cert-manager-webhook-bunny

helm install cert-manager-webhook-bunny \
  deploy/cert-manager-webhook-bunny \
  --namespace cert-manager \
  --create-namespace
```

### 2. Create a Secret with your Bunny.net API access key

Get your **Account API Key** from the [Bunny.net dashboard](https://dash.bunny.net/account/apikey) (not the zone-specific key).

```bash
kubectl create secret generic bunny-credentials \
  --from-literal=accessKey=<YOUR_BUNNY_ACCESS_KEY> \
  --namespace cert-manager
```

> **Note:** The Secret must be in the same namespace as the `Issuer`, or in any namespace for a `ClusterIssuer`. The webhook service account has read access to secrets cluster-wide.

### 3. Create an Issuer or ClusterIssuer

```yaml
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-prod
spec:
  acme:
    server: https://acme-v02.api.letsencrypt.org/directory
    email: your-email@example.com
    privateKeySecretRef:
      name: letsencrypt-prod
    solvers:
      - dns01:
          webhook:
            solverName: bunny
            groupName: acme.bunny.net
            config:
              apiSecretRef:
                name: bunny-credentials
                key: accessKey
```

### 4. Request a Certificate

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: my-cert
  namespace: default
spec:
  secretName: my-cert-tls
  dnsNames:
    - "example.com"
    - "*.example.com"
  issuerRef:
    name: letsencrypt-prod
    kind: ClusterIssuer
```

## Helm Chart Configuration

The following table lists the configurable parameters and their defaults:

| Parameter | Description | Default |
|-----------|-------------|---------|
| `image.repository` | Container image repository | `ghcr.io/cvandesande/cert-manager-webhook-bunny` |
| `image.tag` | Container image tag (defaults to `appVersion` when empty) | `""` |
| `image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `replicaCount` | Number of webhook replicas | `1` |
| `groupName` | ACME DNS01 solver group name | `acme.bunny.net` |
| `certManager.namespace` | Namespace where cert-manager is installed | `cert-manager` |
| `certManager.serviceAccountName` | cert-manager controller service account name | `cert-manager` |
| `resources` | Pod resource requests and limits | `{}` |
| `nodeSelector` | Node selector | `{}` |
| `tolerations` | Pod tolerations | `[]` |
| `affinity` | Pod affinity rules | `{}` |

## Building from Source

```bash
# Build the Docker image
make build

# Lint the Helm chart
make helm-lint

# Render the Helm chart to stdout
make rendered-manifest.yaml
```

## Running Tests

The test suite uses cert-manager's [DNS01 conformance tests](https://github.com/cert-manager/cert-manager/tree/master/test/acme), which:

1. Start a temporary in-process Kubernetes API server (envtest)
2. Create your credentials Secret inside that cluster
3. Call `Present` on the solver — making **real API calls** to Bunny DNS to create a TXT record
4. Verify the record is resolvable by querying Bunny's authoritative nameservers directly
5. Call `CleanUp` to delete the record

### Prerequisites

- A real domain managed by Bunny DNS (e.g. `example.com`)
- Your Bunny.net **Account API Key** (from [dash.bunny.net/account/apikey](https://dash.bunny.net/account/apikey))
- The [kubebuilder test tools](https://go.kubebuilder.io/test-tools/) (etcd + kube-apiserver) — downloaded automatically by `make`

No credential files need to be created — credentials are read entirely from environment variables.
The test skips automatically if either variable is unset, making it safe to run in CI without secrets.

### Required Environment Variables

| Variable | Description |
|----------|-------------|
| `BUNNY_ACCESS_KEY` | Your Bunny.net Account API Key |
| `TEST_ZONE_NAME` | A DNS zone managed in Bunny DNS, with trailing dot (e.g. `example.com.`) |

### Optional Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `TEST_USE_AUTHORITATIVE` | `true` | Query the zone's own authoritative nameservers (Bunny's `kiki.bunny.net` / `coco.bunny.net`). Records are visible immediately after creation. Set to `false` to use a public resolver instead. |
| `TEST_DNS_SERVER` | _(none)_ | Custom DNS server to use when `TEST_USE_AUTHORITATIVE=false` (e.g. `8.8.8.8:53`). |

### Running with Go

`make test` automatically installs `setup-envtest` and the correct Kubernetes binaries:

```bash
export BUNNY_ACCESS_KEY=your-api-key-here
export TEST_ZONE_NAME=example.com.
make test
```

### Running with Docker (no local Go required)

Everything — including downloading depedencies and test binaries — happens inside the container:

```bash
make test-docker BUNNY_ACCESS_KEY=your-api-key-here TEST_ZONE_NAME=example.com.
```

### Important Notes

- The test creates and deletes a real `_acme-challenge` TXT record in your Bunny DNS zone
- In authoritative mode (the default), records are visible immediately on Bunny's nameservers — no propagation delay
- Tests skip gracefully when `BUNNY_ACCESS_KEY` or `TEST_ZONE_NAME` are not set, so the test suite is safe to run in CI environments that don't have credentials configured

## Certbot (bare metal / standalone)

For machines that run [certbot](https://certbot.eff.org/) directly — without Kubernetes — a small standalone binary is provided: **`bunny-certbot-hook`**.

It is used as certbot's `--manual-auth-hook` and `--manual-cleanup-hook` to create and remove the DNS-01 challenge TXT records in Bunny DNS automatically.

### Download

Pre-built binaries for Linux (amd64 and arm64) are attached to every
[GitHub Release](https://github.com/cvandesande/cert-manager-webhook-bunny/releases).

```bash
# Example: download the amd64 binary for v1.1.2
curl -L -o /usr/local/bin/bunny-certbot-hook \
  https://github.com/cvandesande/cert-manager-webhook-bunny/releases/download/v1.1.2/bunny-certbot-hook-linux-amd64
chmod +x /usr/local/bin/bunny-certbot-hook
```

Replace `amd64` with `arm64` on ARM hosts.

### Usage

Store your Bunny.net **Account API Key** (from [dash.bunny.net/account/apikey](https://dash.bunny.net/account/apikey)) in the default location:

```bash
mkdir -p /etc/bunny
echo 'your-bunny-api-key-here' > /etc/bunny/api-key
chmod 600 /etc/bunny/api-key
```

Then run certbot with manual DNS hooks — no extra environment variables needed:

```bash
certbot certonly \
  --manual \
  --preferred-challenges dns \
  --manual-auth-hook    "bunny-certbot-hook present" \
  --manual-cleanup-hook "bunny-certbot-hook cleanup" \
  -d "example.com" \
  -d "*.example.com"
```

Certbot sets `CERTBOT_DOMAIN` and `CERTBOT_VALIDATION` automatically before invoking each hook. The binary auto-discovers the correct Bunny DNS zone, so both apex domains (`example.com`) and subdomains (`sub.example.com`) work without any extra configuration.

For **auto-renewal**, add these lines to `/etc/letsencrypt/renewal/example.com.conf`:

```ini
[renewalparams]
authenticator = manual
manual_auth_hook    = bunny-certbot-hook present
manual_cleanup_hook = bunny-certbot-hook cleanup
manual_public_ip_logging_ok = True
pref_challs = dns-01
```

> `pref_challs = dns-01` is required to prevent certbot from attempting HTTP-01 in addition to DNS-01. Without it, Let's Encrypt may try to reach your server over HTTP and fail if port 80 is not publicly accessible.

### API key lookup order

The binary checks these locations in order and uses the first key found:

| Priority | Source | Notes |
|----------|--------|-------|
| 1 | `BUNNY_API_KEY` env var | Plain key value |
| 2 | `BUNNY_API_KEY_FILE` env var | Path to a file containing the key |
| 3 | `/etc/bunny/api-key` | Default file (recommended) |

### Certbot environment variables (set automatically)

| Variable | Description |
|----------|-------------|
| `CERTBOT_DOMAIN` | Domain being validated (e.g. `example.com`) |
| `CERTBOT_VALIDATION` | Value to place in the TXT record |

### Build from source

**With Go installed** (produces `./bunny-certbot-hook` for the host architecture):
```bash
make build-hook
```

**Without Go** (uses Docker; produces `./bunny-certbot-hook-linux-amd64` and `./bunny-certbot-hook-linux-arm64`):
```bash
make build-hook-docker
```

## Architecture

The webhook implements the cert-manager `webhook.Solver` interface:

- **`Present`** – Creates a TXT DNS record in the specified Bunny DNS zone to satisfy the ACME DNS-01 challenge.
- **`CleanUp`** – Deletes the TXT DNS record once the challenge is complete.
- **`Initialize`** – Sets up the Kubernetes client for reading Secrets.

The webhook is registered as a Kubernetes API extension via an `APIService` resource. cert-manager routes DNS-01 challenge requests to the webhook's API endpoint. TLS for the webhook server is automatically provisioned by cert-manager using a self-signed CA.

## License

[Apache 2.0](LICENSE)
