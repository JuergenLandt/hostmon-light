# syntax=docker/dockerfile:1
ARG GO_VERSION=1.26.10
FROM golang:${GO_VERSION} AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY cmd ./cmd
RUN CGO_ENABLED=0 go test -mod=readonly ./...
RUN CGO_ENABLED=0 go build -mod=readonly -trimpath -ldflags="-s -w" -o /out/hostmon-light ./cmd/hostmon-light

FROM scratch AS runner
COPY --from=build /out/hostmon-light /hostmon-light
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/hostmon-light"]
