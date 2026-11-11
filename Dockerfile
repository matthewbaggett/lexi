FROM golang:1.26 AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /lexi ./cmd/lexi

FROM scratch
COPY --from=build /lexi /lexi
ENTRYPOINT ["/lexi"]
