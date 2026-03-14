FROM golang:1.26-alpine AS build_deps

RUN apk add --no-cache git

WORKDIR /workspace

COPY go.mod .
COPY go.sum .

RUN go mod download

FROM build_deps AS build

COPY . .

RUN CGO_ENABLED=0 go build -o webhook -ldflags '-w -extldflags "-static"' .

# gcr.io/distroless/static-debian13:nonroot is a minimal image with:
#   - CA certificates (required for TLS)
#   - No shell, no package manager — minimal attack surface
#   - Runs as UID 65532 (nonroot) by default
FROM gcr.io/distroless/static-debian13:nonroot

COPY --from=build /workspace/webhook /usr/local/bin/webhook

ENTRYPOINT ["/usr/local/bin/webhook"]
