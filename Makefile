# Atajos (Linux/macOS o Windows con make). En Windows sin make, use los comandos del README.
.PHONY: deps test test-mongo check demo run build docker-up docker-down

deps:
	go mod tidy

test: deps
	go vet ./...
	go test ./...

# Modo demo: sin GitHub ni MongoDB
demo: deps
	GITHUB_MODE=demo STORE=memory COOKIE_SECURE=false \
	BOOTSTRAP_ADMIN_EMAIL=admin@empresa.com BOOTSTRAP_ADMIN_PASSWORD=CambiarEsta2026 \
	go run ./cmd/server

# Prueba de la capa de datos contra un MongoDB real local
test-mongo: deps
	MONGO_TEST_URI=mongodb://localhost:27017 go test ./internal/store -run Mongo -v

# Diagnóstico de configuración, GitHub y MongoDB (lee .env automáticamente)
check: deps
	go run ./cmd/server check

# Inicia la app con la configuración de .env (se lee automáticamente)
run: deps
	go run ./cmd/server

build: deps
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/server ./cmd/server

docker-up:
	docker compose up -d --build

docker-down:
	docker compose down
