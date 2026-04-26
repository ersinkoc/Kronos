# syntax=docker/dockerfile:1

ARG GO_VERSION=1.23

FROM golang:${GO_VERSION}-alpine AS build
WORKDIR /src

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X github.com/kronos/kronos/internal/buildinfo.Version=${VERSION} -X github.com/kronos/kronos/internal/buildinfo.Commit=${COMMIT} -X github.com/kronos/kronos/internal/buildinfo.BuildDate=${BUILD_DATE}" \
    -o /out/kronos ./cmd/kronos

FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /out/kronos /kronos
EXPOSE 8500
ENTRYPOINT ["/kronos"]
CMD ["server", "--listen", "0.0.0.0:8500"]
