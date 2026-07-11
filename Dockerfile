# syntax=docker/dockerfile:1
FROM golang:1.24.5-alpine3.22 AS build
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/kubesqueeze ./cmd/kubesqueeze

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/kubesqueeze /kubesqueeze
USER nonroot:nonroot
ENTRYPOINT ["/kubesqueeze"]
