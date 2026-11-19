.DEFAULT_GOAL := up

COMPOSE       ?= docker compose
APP_URL       ?= http://localhost:8080
HEALTH_TIMEOUT ?= 60

.PHONY: up docker-up down docker-down clean docker-clean restart logs ps status \
        build run test fmt vet tidy smoke test-e2e help

# Target por defecto: con solo escribir `make` se levanta todo.
up:
	@echo "==> Levantando Postgres, Redis y la API con Docker..."
	@$(COMPOSE) up --build -d --wait --wait-timeout $(HEALTH_TIMEOUT)
	@echo "==> Health checks OK. API en $(APP_URL)"
	@echo "==> Útiles: make logs | make test | make smoke | make down"

# Aliases retrocompatibles
docker-up: up

down:
	@$(COMPOSE) down

docker-down: down

clean:
	@$(COMPOSE) down -v --remove-orphans

docker-clean: clean

restart: down up

logs:
	@$(COMPOSE) logs -f

ps status:
	@$(COMPOSE) ps

build:
	go build -o bin/api ./cmd/api

# Ejecuta la API en local (necesita DB/Redis en localhost). Para Docker usa `make`.
run:
	go run ./cmd/api

test:
	@echo "==> Corriendo tests unitarios..."
	go test ./...

fmt:
	gofmt -w .

vet:
	go vet ./...

tidy:
	go mod tidy

smoke:
	./scripts/smoke.sh

test-e2e:
	@echo "=== 1. Limpiando contenedores y volúmenes previos ==="
	$(COMPOSE) down -v --remove-orphans
	@echo "=== 2. Levantando contenedores y esperando health checks ==="
	$(COMPOSE) up --build -d --wait --wait-timeout $(HEALTH_TIMEOUT)
	@echo "=== 3. Ejecutando tests ==="
	go test ./...
	./scripts/smoke.sh
	@echo "=== 4. Borrando contenedores y volúmenes ==="
	$(COMPOSE) down -v --remove-orphans

help:
	@echo "Comandos disponibles:"
	@echo "  make            Levanta Docker + API hasta que los health checks estén OK (target por defecto)"
	@echo "  make test       Corre go test ./..."
	@echo "  make smoke      Corre el smoke test contra $(APP_URL)"
	@echo "  make logs       Sigue los logs de todos los servicios"
	@echo "  make ps         Muestra el estado de los contenedores"
	@echo "  make restart    Reinicia todo"
	@echo "  make down       Baja los contenedores (conserva volúmenes)"
	@echo "  make clean      Baja contenedores y borra volúmenes"
	@echo "  make test-e2e   Test completo en Docker (limpia al final)"
