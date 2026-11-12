FROM --platform=$BUILDPLATFORM golang:1.26 AS build
ARG VERSION=dev
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /lexi ./cmd/lexi

FROM scratch
COPY --from=build /lexi /lexi
ENTRYPOINT ["/lexi"]
