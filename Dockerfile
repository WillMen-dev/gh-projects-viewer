# ---------- Etapa 1: compilación ----------
FROM golang:1.24-alpine AS build
WORKDIR /src
RUN apk add --no-cache git ca-certificates
COPY . .
# go mod tidy genera/actualiza go.sum y descarga dependencias verificando sus checksums.
RUN go mod tidy && go test ./... && \
    CGO_ENABLED=0 GOOS=linux go build -tags timetzdata -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

# ---------- Etapa 2: imagen final mínima ----------
# distroless: sin shell ni gestor de paquetes (menor superficie de ataque), usuario no root.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/server /app/server
USER nonroot:nonroot
EXPOSE 8080
ENV APP_ADDR=:8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD ["/app/server", "healthcheck"]
ENTRYPOINT ["/app/server"]
