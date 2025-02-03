FROM golang:1.23-alpine3.21 AS builder

RUN apk add --no-cache bash git gcc musl-dev

WORKDIR /src
COPY . .

RUN . ./scripts/version
RUN --mount=type=cache,id=gomod,target=/go/pkg/mod \
    --mount=type=cache,id=gobuild,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -ldflags "-X main.VERSION=$VERSION -extldflags -static -s" -o /bin/helm-controller

FROM scratch AS binary
COPY --from=builder /bin/helm-controller /bin/

# Dev stage for package, testing, and validation
FROM golang:1.23-alpine3.21 AS dev
ARG ARCH
ENV ARCH=$ARCH
RUN apk add --no-cache bash git gcc musl-dev curl
RUN GOPROXY=direct go install golang.org/x/tools/cmd/goimports@gopls/v0.16.2
RUN if [ "${ARCH}" != "arm" ]; then \
    curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s v1.63.4; \
    fi
RUN if [ "${ARCH}" = "amd64" ]; then \
    go install sigs.k8s.io/kustomize/kustomize/v4@v4.5.7; \
    fi

WORKDIR /src
COPY go.mod go.sum pkg/ main.go ./
RUN go mod download
COPY . .

FROM dev AS package
RUN ./scripts/package

FROM scratch AS artifacts
COPY --from=package /src/dist/artifacts /dist/artifacts

FROM alpine:3.21 AS production
COPY bin/helm-controller /usr/bin/
CMD ["helm-controller"]