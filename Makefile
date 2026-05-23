COMPOSE ?= docker compose
COMPOSE_LOCAL = $(COMPOSE) -f docker-compose.yml -f docker-compose.local-build.yml

ifeq ($(OS),Windows_NT)
BUILD_BACKEND = powershell -ExecutionPolicy Bypass -File backend/bin/build-linux.ps1
else
BUILD_BACKEND = ./backend/bin/build-linux.sh
endif

.PHONY: build-backend up down logs

build-backend:
	$(BUILD_BACKEND)

up: build-backend
	$(COMPOSE_LOCAL) up -d --build

down:
	$(COMPOSE) down

logs:
	$(COMPOSE) logs -f
