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
    curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- v2.7.2; \
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
