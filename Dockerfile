# Dashboard
FROM node:24-alpine AS web
WORKDIR /web
COPY web/package*.json ./
RUN npm ci
COPY web ./
RUN npm run build

# One static binary with the dashboard embedded
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY web/embed.go ./web/
COPY --from=web /web/dist ./web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /llmproxy ./cmd/llmproxy

# Runtime: the binary, CA certificates and a non-root user — no shell, no package manager.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /llmproxy /llmproxy
EXPOSE 8000 8081
ENTRYPOINT ["/llmproxy"]
