OS ?= $(shell uname -s | tr '[:upper:]' '[:lower:]')
ARCH ?= $(shell uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')

IMAGE_NAME := "ghcr.io/cvandesande/cert-manager-webhook-bunny"
IMAGE_TAG := "latest"

OUT := $(shell pwd)/_out

# Kubernetes version for the envtest binaries (see: setup-envtest list).
KUBE_VERSION ?= 1.32

$(shell mkdir -p "$(OUT)")

# Run conformance tests locally (requires Go; installs setup-envtest automatically).
# Usage: BUNNY_ACCESS_KEY=xxx TEST_ZONE_NAME=example.com. make test
.PHONY: test
test:
	go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest 2>&1
	@ENVTEST_BIN=$$(setup-envtest use $(KUBE_VERSION) --bin-dir _test/envtest -p path); \
	 TEST_ASSET_ETCD=$$ENVTEST_BIN/etcd \
	 TEST_ASSET_KUBE_APISERVER=$$ENVTEST_BIN/kube-apiserver \
	 TEST_ASSET_KUBECTL=$$ENVTEST_BIN/kubectl \
	 go test -v .

# Run conformance tests inside Docker (no local Go required).
# Usage: make test-docker BUNNY_ACCESS_KEY=xxx TEST_ZONE_NAME=example.com.
.PHONY: test-docker
test-docker:
	docker run --rm \
	    -v "$(CURDIR)":/workspace \
	    -w /workspace \
	    -e BUNNY_ACCESS_KEY=$(BUNNY_ACCESS_KEY) \
	    -e TEST_ZONE_NAME=$(TEST_ZONE_NAME) \
	    golang:1.26-alpine \
	    sh -c 'apk add --no-cache git 1>/dev/null 2>&1 && \
	           go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest 2>&1 && \
	           ENVTEST_BIN=$$(setup-envtest use $(KUBE_VERSION) --bin-dir /tmp/envtest -p path) && \
	           TEST_ASSET_ETCD=$$ENVTEST_BIN/etcd \
	           TEST_ASSET_KUBE_APISERVER=$$ENVTEST_BIN/kube-apiserver \
	           TEST_ASSET_KUBECTL=$$ENVTEST_BIN/kubectl \
	           go test -buildvcs=false -v .'

.PHONY: clean
clean:
	rm -Rf _test _out

# Build for the local architecture only (fast, for development/testing).
.PHONY: build
build:
	docker build -t "$(IMAGE_NAME):$(IMAGE_TAG)" .

# Build and push a multi-platform image (linux/amd64 + linux/arm64).
# Requires a buildx builder with multi-platform support (e.g. docker buildx create).
# Usage: make build-multiplatform IMAGE_NAME=registry.example.com/my-image IMAGE_TAG=latest
.PHONY: build-multiplatform
build-multiplatform:
	docker buildx build \
	    --platform linux/amd64,linux/arm64 \
	    --tag "$(IMAGE_NAME):$(IMAGE_TAG)" \
	    --push \
	    .

# ──────────────────────────────────────────────────────────────────────────────
# certbot hook binary
# ──────────────────────────────────────────────────────────────────────────────

HOOK_BINARY := bunny-certbot-hook

# Build the certbot hook binary for the host architecture (requires Go).
# Output: ./bunny-certbot-hook
.PHONY: build-hook
build-hook:
	GOOS=linux GOARCH=$(ARCH) go build \
	    -trimpath \
	    -ldflags="-s -w" \
	    -o $(HOOK_BINARY) \
	    ./cmd/certbot-hook

# Build the certbot hook binary inside Docker (no local Go required).
# Produces two binaries: ./bunny-certbot-hook-linux-amd64 and ./bunny-certbot-hook-linux-arm64
.PHONY: build-hook-docker
build-hook-docker:
	docker run --rm \
	    -v "$(CURDIR)":/workspace \
	    -w /workspace \
	    golang:1.26-alpine \
	    sh -c 'apk add --no-cache git 1>/dev/null 2>&1 && \
	           GOOS=linux GOARCH=amd64 go build -buildvcs=false -trimpath -ldflags="-s -w" \
	               -o bunny-certbot-hook-linux-amd64 ./cmd/certbot-hook && \
	           GOOS=linux GOARCH=arm64 go build -buildvcs=false -trimpath -ldflags="-s -w" \
	               -o bunny-certbot-hook-linux-arm64 ./cmd/certbot-hook'
	@echo "Built: bunny-certbot-hook-linux-amd64 bunny-certbot-hook-linux-arm64"

# ──────────────────────────────────────────────────────────────────────────────
# Package and push the Helm chart to an OCI registry.
# Usage: make helm-push-oci CHART_VERSION=1.0.0 OCI_REGISTRY=oci://ghcr.io/cvandesande/charts
OCI_REGISTRY ?= oci://ghcr.io/cvandesande/charts
CHART_VERSION ?= $(shell grep '^version:' deploy/cert-manager-webhook-bunny/Chart.yaml | awk '{print $$2}')

.PHONY: helm-push-oci
helm-push-oci:
	helm package deploy/cert-manager-webhook-bunny --version "$(CHART_VERSION)"
	helm push cert-manager-webhook-bunny-$(CHART_VERSION).tgz $(OCI_REGISTRY)
	rm -f cert-manager-webhook-bunny-$(CHART_VERSION).tgz

.PHONY: helm-lint
helm-lint:
	helm lint deploy/cert-manager-webhook-bunny

.PHONY: rendered-manifest.yaml
rendered-manifest.yaml:
	helm template \
	    cert-manager-webhook-bunny \
	    --set image.repository=$(IMAGE_NAME) \
	    --set image.tag=$(IMAGE_TAG) \
	    deploy/cert-manager-webhook-bunny > "$(OUT)/rendered-manifest.yaml"
