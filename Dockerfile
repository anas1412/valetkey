# Both build stages run natively on the build machine ($BUILDPLATFORM): the dashboard
# is the same on every platform, and Go cross-compiles, so multi-arch images need no
# CPU emulation.

# Dashboard
FROM --platform=$BUILDPLATFORM node:24-alpine AS web
WORKDIR /web
COPY web/package*.json ./
RUN npm ci
COPY web ./
RUN npm run build

# One static binary with the dashboard embedded
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY web/embed.go ./web/
COPY --from=web /web/dist ./web/dist
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /valetkey ./cmd/valetkey

# Runtime: the binary, CA certificates and a non-root user — no shell, no package manager.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /valetkey /valetkey
EXPOSE 8000 8081
ENTRYPOINT ["/valetkey"]
