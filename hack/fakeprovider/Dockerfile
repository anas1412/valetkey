# Demo-only image: a fake LLM provider and traffic generator (see hack/demo.sh).
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY hack/fakeprovider ./hack/fakeprovider
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /fakeprovider ./hack/fakeprovider

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /fakeprovider /fakeprovider
ENTRYPOINT ["/fakeprovider"]
