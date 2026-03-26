FROM golang:1.25-alpine3.23 AS builder

RUN apk add --no-cache bash git gcc musl-dev

WORKDIR /src
COPY . .

RUN GOPROXY=direct go install sigs.k8s.io/controller-tools/cmd/controller-gen@v0.19.0
RUN GOPROXY=direct go install github.com/elastic/crd-ref-docs@v0.2.0

RUN go generate ./...

RUN --mount=type=cache,id=gomod,target=/go/pkg/mod \
    --mount=type=cache,id=gobuild,target=/root/.cache/go-build \
    ./scripts/build

FROM scratch AS binary
COPY --from=builder /src/bin/helm-controller /bin/

# Dev stage for package, testing, and validation
FROM golang:1.25-alpine3.23 AS dev
ARG ARCH
ENV ARCH=$ARCH
RUN apk add --no-cache bash git curl
RUN if [ "${ARCH}" != "arm" ]; then \
    GOLANGCI_VERSION=v2.7.2 && \
    case "${ARCH}" in \
        amd64) GOLANGCI_SHA256="ce46a1f1d890e7b667259f70bb236297f5cf8791a9b6b98b41b283d93b5b6e88" ;; \
        arm64) GOLANGCI_SHA256="7028e810837722683dab679fb121336cfa303fecff39dfe248e3e36bc18d941b" ;; \
        *) echo "Unsupported architecture for golangci-lint: ${ARCH}" && exit 1 ;; \
    esac && \
    cd /tmp && \
    curl -fsSL "https://github.com/golangci/golangci-lint/releases/download/${GOLANGCI_VERSION}/golangci-lint-${GOLANGCI_VERSION#v}-linux-${ARCH}.tar.gz" -o golangci-lint.tar.gz && \
    echo "${GOLANGCI_SHA256}  golangci-lint.tar.gz" | sha256sum -c - && \
    tar --strip-components=1 -xzf golangci-lint.tar.gz -C /usr/local/bin golangci-lint-${GOLANGCI_VERSION#v}-linux-${ARCH}/golangci-lint && \
    rm -f /tmp/golangci-lint.tar.gz; \
    fi
RUN if [ "${ARCH}" = "amd64" ]; then \
    go install sigs.k8s.io/kustomize/kustomize/v5@v5.8.1; \
    fi

WORKDIR /src
COPY go.mod go.sum pkg/ main.go ./
RUN go mod download
COPY . .

FROM dev AS package
RUN ./scripts/package

FROM scratch AS artifacts
COPY --from=package /src/dist/artifacts /dist/artifacts

FROM scratch AS crds
COPY --from=builder /src/pkg/crds/yaml/generated/ /
COPY --from=builder /src/doc/helmchart.md /tmp_doc/

FROM alpine:3.23 AS production
COPY bin/helm-controller /usr/bin/
CMD ["helm-controller"]
