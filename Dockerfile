FROM golang:1.22 AS builder
WORKDIR /workspace
ARG TARGETOS=linux
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -ldflags="-s -w -X k8s-insight-controller/internal/version.Version=${VERSION} -X k8s-insight-controller/internal/version.Commit=${COMMIT} -X k8s-insight-controller/internal/version.Date=${DATE}" \
    -o manager ./cmd/manager

FROM gcr.io/distroless/static:nonroot
ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown
LABEL org.opencontainers.image.title="k8s-insight-controller" \
      org.opencontainers.image.description="Kubernetes controller that generates LLM-powered cluster insight reports." \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}" \
      org.opencontainers.image.created="${DATE}" \
      org.opencontainers.image.licenses="Apache-2.0"
WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532
ENTRYPOINT ["/manager"]
