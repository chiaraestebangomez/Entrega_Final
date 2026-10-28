.PHONY: build run test fmt vet tidy docker-up docker-down docker-clean smoke test-e2e

build:
	go build -o bin/api ./cmd/api

run:
	go run ./cmd/api

test:
	go test ./...

fmt:
	gofmt -w .

vet:
	go vet ./...

tidy:
	go mod tidy

docker-up:
	docker compose up --build -d

docker-down:
	docker compose down

# Limpieza completa de contenedores, volúmenes y huérfanos
docker-clean:
	docker compose down -v --remove-orphans

smoke:
	./scripts/smoke.sh

# Target de prueba completa en entorno aislado con Docker
test-e2e:
	@echo "=== 1. Limpiando contenedores y volúmenes previos ==="
	docker compose down -v --remove-orphans
	@echo "=== 2. Levantando contenedores de Docker ==="
	docker compose up --build -d
	@echo "=== Esperando a que la base de datos y la API estén listas ==="
	@until curl -s http://localhost:8080/readyz > /dev/null; do \
		echo "Esperando respuesta de la API/BD en http://localhost:8080/readyz..."; \
		sleep 2; \
	done
	@echo "=== 3. Ejecutando tests ==="
	-go test ./...
	-./scripts/smoke.sh
	@echo "=== 4. Tareas posteriores: Borrando contenedores y volúmenes ==="
	docker compose down -v --remove-orphans
