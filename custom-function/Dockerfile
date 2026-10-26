# syntax=docker/dockerfile:1

# CI sets GO_VERSION so the image is built with the same toolchain that ran the tests.
ARG GO_VERSION=1

# Build on the host's platform and cross-compile for the target one: no QEMU, much faster multi-arch builds.
FROM --platform=${BUILDPLATFORM} golang:${GO_VERSION} AS build

WORKDIR /fn

# CGo is not needed; a static binary runs on the distroless static image below.
ENV CGO_ENABLED=0

# Download modules in their own cached step. The type=bind mount lends the build context read-only.
RUN --mount=type=bind,target=. --mount=type=cache,target=/go/pkg/mod go mod download

ARG TARGETOS
ARG TARGETARCH

RUN --mount=type=bind,target=. \
    --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /function .

FROM gcr.io/distroless/static-debian12:nonroot AS image
WORKDIR /
COPY --from=build /function /function
EXPOSE 9443
USER nonroot:nonroot
ENTRYPOINT ["/function"]
