# syntax=docker/dockerfile:1.7
FROM golang:1.26-alpine AS build
WORKDIR /src
ENV CGO_ENABLED=0 GOFLAGS=-trimpath
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    go build -ldflags="-s -w -X main.version=${VERSION}" -o /out/bhagirath ./cmd/bhagirath

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/bhagirath /app/bhagirath
COPY config /app/config
COPY backtest /app/backtest
USER nonroot:nonroot
EXPOSE 8080
ENV BHAGIRATH_ADDR=:8080
HEALTHCHECK NONE
ENTRYPOINT ["/app/bhagirath"]
CMD ["serve"]
