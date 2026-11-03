# Built on the runner's own architecture and cross-compiled by Go, so no emulation. CGO is off:
# nothing here needs it.
FROM --platform=$BUILDPLATFORM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG TARGETOS TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/salvo ./cmd/salvo

# Distroless, non-root. The migrations are embedded in the binary: `salvo migrate`.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/salvo /salvo
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/salvo"]
