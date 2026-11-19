FROM golang:1.26-alpine3.24 AS builder

RUN apk add --no-cache bash git gcc musl-dev

WORKDIR /src
COPY . .

RUN --mount=type=cache,id=gomod,target=/go/pkg/mod \
    --mount=type=cache,id=gobuild,target=/root/.cache/go-build \
    GOPROXY=direct go install sigs.k8s.io/controller-tools/cmd/controller-gen@257e3a04698a16ea834c49f457de7704474f9a74; \
    GOPROXY=direct go install github.com/elastic/crd-ref-docs@7de989285647936ac62ea1ff8887e25e0056bc58; \
    go generate ./...; \
    ./scripts/build

FROM scratch AS binary
COPY --from=builder /src/bin/helm-controller /bin/

# Dev stage for package, testing, and validation
FROM golang:1.26-alpine3.24 AS dev
ARG ARCH
ENV ARCH=$ARCH
RUN apk add --no-cache bash git curl
RUN if [ "${ARCH}" != "arm" ]; then \
    GOLANGCI_VERSION=v2.13.2 && \
    case "${ARCH}" in \
        amd64) GOLANGCI_SHA256="2277d43b98ec0054280f2ac26b53268bae97682444678a59a657dd565da021d6" ;; \
        arm64) GOLANGCI_SHA256="a2a4e0065aa41be71f7c5ac90f271b61751331e5d04314e62afe4027855f0893" ;; \
        *) echo "Unsupported architecture for golangci-lint: ${ARCH}" && exit 1 ;; \
    esac && \
    cd /tmp && \
    curl -fsSL "https://github.com/golangci/golangci-lint/releases/download/${GOLANGCI_VERSION}/golangci-lint-${GOLANGCI_VERSION#v}-linux-${ARCH}.tar.gz" -o golangci-lint.tar.gz && \
    echo "${GOLANGCI_SHA256}  golangci-lint.tar.gz" | sha256sum -c - && \
    tar --strip-components=1 -xzf golangci-lint.tar.gz -C /usr/local/bin golangci-lint-${GOLANGCI_VERSION#v}-linux-${ARCH}/golangci-lint && \
    rm -f /tmp/golangci-lint.tar.gz; \
    fi
RUN --mount=type=cache,id=gomod,target=/go/pkg/mod \
    --mount=type=cache,id=gobuild,target=/root/.cache/go-build \
    if [ "${ARCH}" = "amd64" ]; then \
      go install sigs.k8s.io/kustomize/kustomize/v5@1155ccbe3c1fc56cbbd82847899f86c7b824e005; \
    fi

WORKDIR /src
COPY go.mod go.sum pkg/ main.go ./
RUN --mount=type=cache,id=gomod,target=/go/pkg/mod \
    --mount=type=cache,id=gobuild,target=/root/.cache/go-build \
    go mod download
COPY . .

FROM dev AS package
RUN ./scripts/package

FROM scratch AS artifacts
COPY --from=package /src/dist/artifacts /dist/artifacts

FROM scratch AS crds
COPY --from=builder /src/pkg/crds/yaml/generated/ /
COPY --from=builder /src/doc/helmchart.md /tmp_doc/

FROM alpine:3.24 AS production
# same as klipper-helm
RUN apk --no-cache upgrade && \
    apk del apk-tools zlib libcrypto3 libssl3
COPY bin/helm-controller /usr/bin/
CMD ["helm-controller"]
