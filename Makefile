MAKEFILE_PATH := $(abspath $(firstword $(MAKEFILE_LIST)))
CUR_DIR := $(patsubst %/,%, $(dir $(MAKEFILE_PATH)))
BUILD_DIR := $(CUR_DIR)/.build
APP_EXECUTABLE_DIR := $(BUILD_DIR)/bin

# Путь к go (при вызове make из терминала PATH передаётся в make)
GO ?= $(shell command -v go 2>/dev/null || echo "go")
# Пути к линтерам (go install ставит в GOPATH/bin или HOME/go/bin)
STATICCHECK ?= $(shell command -v staticcheck 2>/dev/null || echo "$$HOME/go/bin/staticcheck")
GOLANGCI_LINT ?= $(shell command -v golangci-lint 2>/dev/null || echo "$$HOME/go/bin/golangci-lint")

# заглушает вывод make
# MAKEFLAGS+=silent # временно отключено, пока не сделана задача BZ-26

# Сгенерировать моки по директивам go:generate.
mocks:
	@echo "> generating mocks..."
	go generate ./...
	@echo "> mocks generated successfully"

# Сгенерировать Swagger-документацию из кода обработчиков.
swag:
	@echo "> generating swagger documentation..."
	swag init -g cmd/app/main.go --md ./docs --parseInternal  --parseDependency --parseDepth 2 
	@echo "> swagger documentation generated successfully"

# Установить линтеры и создать локальные сертификаты Vault.
init:
	@echo "> initializing..."
	@make install-linters
	@make certs

# Создать комплект локальных сертификатов, не перезаписывая существующие ключи.
certs:
	@echo "> generating certs..."
	@mkdir -p vault
	@if [ -f vault/ca.crt ] || [ -f vault/private-key.pem ] || [ -f vault/full-chain.pem ]; then \
		echo "Error: Certificate files already exist. Remove them manually or use 'make clean-certs' to regenerate."; \
		exit 1; \
	fi
	@make _generate-ca-cert
	@make _generate-server-cert
	@make _generate-client-cert
	@make _cleanup-temp-files
	@echo "> certs generated successfully"

# Создать временный локальный центр сертификации для сертификатов Vault.
_generate-ca-cert:
	@echo "  > generating CA certificate..."
	@printf '%s\n' \
		'[req]' \
		'distinguished_name = req_distinguished_name' \
		'x509_extensions = v3_ca' \
		'prompt = no' \
		'' \
		'[req_distinguished_name]' \
		'C = RU' \
		'ST = State' \
		'L = City' \
		'O = Organization' \
		'CN = Vault CA' \
		'' \
		'[v3_ca]' \
		'basicConstraints = critical,CA:true' \
		'keyUsage = critical, keyCertSign, cRLSign' > vault/ca.conf
	@openssl req -x509 -newkey rsa:4096 -keyout vault/ca.key -out vault/ca.crt \
		-days 365 -nodes -config vault/ca.conf -extensions v3_ca

# Создать серверный сертификат Vault для localhost.
_generate-server-cert:
	@echo "  > generating server certificate..."
	@printf '%s\n' \
		'[req]' \
		'distinguished_name = req_distinguished_name' \
		'req_extensions = v3_req' \
		'prompt = no' \
		'' \
		'[req_distinguished_name]' \
		'C = RU' \
		'ST = State' \
		'L = City' \
		'O = Organization' \
		'CN = localhost' \
		'' \
		'[v3_req]' \
		'keyUsage = critical, digitalSignature, keyEncipherment' \
		'extendedKeyUsage = serverAuth' \
		'subjectAltName = @alt_names' \
		'' \
		'[alt_names]' \
		'DNS.1 = localhost' \
		'DNS.2 = *.localhost' \
		'IP.1 = 127.0.0.1' \
		'IP.2 = ::1' > vault/server.conf
	@openssl req -newkey rsa:4096 -keyout vault/private-key.pem -out vault/server.csr \
		-nodes -config vault/server.conf
	@openssl x509 -req -in vault/server.csr -CA vault/ca.crt -CAkey vault/ca.key \
		-CAcreateserial -out vault/full-chain.pem -days 365 \
		-extensions v3_req -extfile vault/server.conf

# Создать клиентский сертификат для локального подключения к Vault.
_generate-client-cert:
	@echo "  > generating client certificate..."
	@printf '%s\n' \
		'[req]' \
		'distinguished_name = req_distinguished_name' \
		'req_extensions = v3_req' \
		'prompt = no' \
		'' \
		'[req_distinguished_name]' \
		'C = RU' \
		'ST = State' \
		'L = City' \
		'O = Organization' \
		'CN = vault-client' \
		'' \
		'[v3_req]' \
		'keyUsage = critical, digitalSignature, keyEncipherment' \
		'extendedKeyUsage = clientAuth' \
		'subjectAltName = @alt_names' \
		'' \
		'[alt_names]' \
		'DNS.1 = localhost' \
		'IP.1 = 127.0.0.1' > vault/client.conf
	@openssl req -newkey rsa:4096 -keyout vault/client.key -out vault/client.csr \
		-nodes -config vault/client.conf
	@openssl x509 -req -in vault/client.csr -CA vault/ca.crt -CAkey vault/ca.key \
		-CAcreateserial -out vault/client.crt -days 365 \
		-extensions v3_req -extfile vault/client.conf

# Удалить промежуточные конфиги, запросы и ключ локального центра сертификации.
_cleanup-temp-files:
	@echo "  > cleaning up temporary files..."
	@rm -f vault/ca.conf vault/server.conf vault/client.conf \
		vault/server.csr vault/client.csr vault/ca.key vault/ca.srl

# Установить инструменты статической проверки Go.
install-linters:
	@echo "> installing linters..."
	$(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
	@echo "> golangci-lint installed successfully"
	$(GOLANGCI_LINT) --version
	
	@echo "> installing staticcheck"
	$(GO) install honnef.co/go/tools/cmd/staticcheck@latest
	@echo "> staticcheck installed successfully"
	$(STATICCHECK) --version
	
	@echo "> linters installed successfully"

# Выполнить go vet, staticcheck и golangci-lint.
lint:
	@echo "> linting..."
	$(GO) vet ./...
	$(STATICCHECK) ./...
	$(GOLANGCI_LINT) run ./...
	@echo "> linting successfully finished"

# Запустить Go-тесты с проверкой гонок и покрытием.
test:
	@echo "> testing..."
	$(GO) test -cover -gcflags="-l" -race -v ./...
	@echo "> successfully finished"

# Последовательно проверить зависимости, стиль, тесты и сборку.
all:	
	@make check
	@make lint
	@make test
	@make build

# Проверить зависимости, go vet и наличие тестовых файлов.
check:
	@echo "> checking..."
	@make check-go-mod
	go vet ./...
	@make check-test-files
	@echo "> check successfully finished"

# Проверить контрольные суммы зависимостей Go.
check-go-mod:
	@echo "> checking go.mod..."
	$(GO) mod verify
	@echo "> go.mod checked successfully"

# Проверить наличие требуемых тестовых файлов в пакетах.
check-test-files:
	@echo "> checking test files..."
	./scripts/check-test-files.sh $$($(GO) list -f '{{.Dir}}' ./...)
	@echo "> test files checked successfully"

# Собрать бинарник и записать версию, дату сборки и Git-коммит через ldflags.
build:
	@echo " > building..."
	@mkdir -p "$(BUILD_DIR)/bin"
	@VERSION=$$(git describe --tags --always --dirty); \
	BUILD_DATE=$$(date -u +%Y%m%d-%H%M%SZ); \
	GIT_COMMIT=$$(git rev-parse --short HEAD); \
	go build -trimpath \
	-ldflags "-s -w -X main.Version=$$VERSION -X main.BuildDate=$$BUILD_DATE -X main.GitCommit=$$GIT_COMMIT" \
	-o "$(BUILD_DIR)/bin/" ./cmd/...
	@echo " > successfully built"

# Пересобрать сервис и запустить локальный бинарник.
run:
	@make build
	$(APP_EXECUTABLE_DIR)/app

# DOCKER

# VAULT
# Поднять Vault через прежний локальный Compose и показать его журнал.
start-vault:
	@echo "> starting vault..."
	docker-compose up -d vault
	@echo "> vault started successfully"
	docker logs auth-service-vault-$(DB)

# Остановить Vault в прежнем локальном окружении.
stop-vault:
	@echo "> stopping vault..."
	docker-compose stop vault
	@echo "> vault stopped successfully"

# Перезапустить Vault в прежнем локальном окружении.
restart-vault:
	@echo "> restarting vault..."
	@make stop-vault
	@make start-vault
	@echo "> vault restarted successfully"

# REDIS
# Поднять Redis через прежний локальный Compose и показать его журнал.
start-redis:
	@echo "> starting redis..."
	docker-compose up -d redis
	@echo "> redis started successfully"
	docker logs auth-service-redis-$(DB)

# Остановить Redis в прежнем локальном окружении.
stop-redis:
	@echo "> stopping redis..."
	docker-compose stop redis
	@echo "> redis stopped successfully"

# Перезапустить Redis в прежнем локальном окружении.
restart-redis:
	@echo "> restarting redis..."
	@make stop-redis
	@make start-redis
	@echo "> redis restarted successfully"

.PHONY: mocks swag lint test all run init install-linters check check-go-mod start-vault stop-vault restart-vault certs start-redis stop-redis restart-redis

-include .env

export POSTGRES_USER
export POSTGRES_PASSWORD
export POSTGRES_DB
export POSTGRES_HOST
export POSTGRES_PORT

DB_URL=postgresql://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@$(POSTGRES_HOST):$(POSTGRES_PORT)/$(POSTGRES_DB)?sslmode=disable&search_path=public

# Вручную применить миграции схемы приложения к БД из .env; это не тестовые данные.
migrate-up:
	migrate -path ./migration -database "$(DB_URL)" -verbose up

# Вручную откатить одну миграцию схемы приложения в БД из .env.
migrate-down:
	migrate -path ./migration -database "$(DB_URL)" -verbose down 1

.PHONY: migrate-up migrate-down add-test-user remove-test-user testdata-up testdata-down

RABBITMQ ?= true

# Базовые сервисы (всегда)
BASE_SERVICES := postgres vault redis open-fga

# Условные сервисы
ifeq ($(RABBITMQ),true)
    ALL_SERVICES := $(BASE_SERVICES) rabbitmq
else
    ALL_SERVICES := $(BASE_SERVICES)
endif

# Подготовить зависимости локального окружения сервиса из docker-compose.yaml.
containers-up:
	@echo "🚀 Starting services: $(ALL_SERVICES)"
	docker compose -f docker-compose.yaml --profile init run --rm setup-vault
	docker compose -f docker-compose.yaml --profile init run --rm fga-migrate
	docker compose -f docker-compose.yaml up -d postgres-fga open-fga
ifeq ($(RABBITMQ),true)
	docker compose -f docker-compose.yaml --profile init run --rm rabbitmq-setup
endif
	docker compose up -d $(ALL_SERVICES)

# Удалить контейнеры и сеть локального окружения сервиса.
containers-down:
	docker compose down

# Удобные алиасы
# Короткое имя containers-up для локального окружения сервиса.
up: containers-up
# Короткое имя containers-down для локального окружения сервиса.
down: containers-down

.PHONY: containers-up containers-down up down

# DOCKER
-include .env
# Настройки по умолчанию, можно переопределять через env
REGISTRY ?= docker.io
IMAGE_TAG ?= latest                   # или $(shell git rev-parse --short HEAD)
FULL_IMAGE := $(REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)

.PHONY: docker-build docker-push docker-login
# Собрать FULL_IMAGE с обязательными метаданными версии, даты и коммита.
docker-build:
	@VERSION=$$(git describe --tags --always --dirty); \
	BUILD_DATE=$$(date -u +%Y%m%d-%H%M%SZ); \
	GIT_COMMIT=$$(git rev-parse --short HEAD); \
	echo "> docker-build with VERSION=$$VERSION BUILD_DATE=$$BUILD_DATE GIT_COMMIT=$$GIT_COMMIT"; \
	docker build \
		-f Dockerfile \
		--build-arg VERSION=$$VERSION \
		--build-arg BUILD_DATE=$$BUILD_DATE \
		--build-arg GIT_COMMIT=$$GIT_COMMIT \
		-t $(FULL_IMAGE) .

# Опубликовать ранее собранный FULL_IMAGE в выбранном реестре.
docker-push:
	docker push $(FULL_IMAGE)

# Опционально: логин, чтобы не писать руками
# Выполнить интерактивный вход в выбранный реестр образов.
docker-login:
	@echo "Logging in to $(REGISTRY)..."
	docker login $(REGISTRY)

# Запустить прежний сценарий docker-compose.tests.yaml; новый запуск — integration-run.
docker-tests-up:
	docker compose -f docker-compose.tests.yaml --profile init run --rm setup-vault
	docker compose -f docker-compose.tests.yaml --profile init run --rm rabbitmq-setup
	docker compose -f docker-compose.tests.yaml --profile init run --rm fga-migrate
	docker compose -f docker-compose.tests.yaml up --build --abort-on-container-exit --exit-code-from pytests --remove-orphans --force-recreate pytests

# Удалить контейнеры прежнего сценария docker-compose.tests.yaml.
docker-tests-down:
	docker compose -f docker-compose.tests.yaml down --remove-orphans

# Выполнить прежний сценарий и убрать его только при успехе команды up.
docker-tests:
	@$(MAKE) docker-tests-up && $(MAKE) docker-tests-down

.PHONY: docker-tests-up docker-tests-down docker-tests

# Общий запуск тестов; локально исходники pytests берутся из соседнего репозитория.
TESTKIT_DIR ?= $(CURDIR)/testkit
PYTESTS_DIR ?= $(abspath ../pytests)
export PYTESTS_DIR
export INTEGRATION_ROOT := $(CURDIR)
export INTEGRATION_CONTEXT := auth-service
# integration-up: подготовить контейнерный стенд testkit; для IDE используйте containers-up.
# integration-run: полный контейнерный цикл; для IDE — готовность, данные и тесты без уборки.
# integration-seed: дождаться сервиса и подготовить данные загрузчиком фикстуры без тестов.
# integration-test: запустить pytest с автоматической подготовкой данных фикстурой.
# integration-rabbitmq-setup: повторить настройку очередей отдельно от загрузки SQL.
# integration-logs: показать логи контейнеров своего стенда; локально RUN_ID не нужен.
# integration-down: удалить контейнеры, тома, сеть и временные файлы своего прогона.
.PHONY: integration-up integration-run integration-seed integration-test integration-down integration-logs integration-rabbitmq-setup
# $@ — запрошенная цель; -C выбирает каталог testkit и его Makefile.
# Например: make integration-seed → make -C ./testkit integration-seed
# → python3 scripts/integration.py seed (реализация находится в testkit).
# @ скрывает только саму команду make, вывод Python/pytest остаётся виден.
integration-up integration-run integration-seed integration-test integration-down integration-logs integration-rabbitmq-setup:
	@$(MAKE) -C "$(TESTKIT_DIR)" $@
