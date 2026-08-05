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
    GOLANGCI_VERSION=v2.12.2 && \
    case "${ARCH}" in \
        amd64) GOLANGCI_SHA256="8df580d2670fed8fa984aac0507099af8df275e665215f5c7a2ae3943893a553" ;; \
        arm64) GOLANGCI_SHA256="sha256:44cd40a8c76c86755375adfeea52cfd3533cb43d7bd647771e0ae065e166df3a" ;; \
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
      go install sigs.k8s.io/kustomize/kustomize/v5@9790a1c3efd2fd35f1b768d495112834176581c1; \
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
COPY bin/helm-controller /usr/bin/
CMD ["helm-controller"]
