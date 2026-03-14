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

mocks:
	@echo "> generating mocks..."
	go generate ./...
	@echo "> mocks generated successfully"

swag:
	@echo "> generating swagger documentation..."
	swag init -g cmd/app/main.go --md ./docs --parseInternal  --parseDependency --parseDepth 2 
	@echo "> swagger documentation generated successfully"

init:
	@echo "> initializing..."
	@make install-linters
	@make certs

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

_cleanup-temp-files:
	@echo "  > cleaning up temporary files..."
	@rm -f vault/ca.conf vault/server.conf vault/client.conf \
		vault/server.csr vault/client.csr vault/ca.key vault/ca.srl

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

lint:
	@echo "> linting..."
	$(GO) vet ./...
	$(STATICCHECK) ./...
	$(GOLANGCI_LINT) run ./...
	@echo "> linting successfully finished"

test:
	@echo "> testing..."
	$(GO) test -cover -gcflags="-l" -race -v ./...
	@echo "> successfully finished"

all:	
	@make check
	@make lint
	@make test
	@make build

check:
	@echo "> checking..."
	@make check-go-mod
	go vet ./...
	@make check-test-files
	@echo "> check successfully finished"

check-go-mod:
	@echo "> checking go.mod..."
	$(GO) mod verify
	@echo "> go.mod checked successfully"

check-test-files:
	@echo "> checking test files..."
	./scripts/check-test-files.sh $$($(GO) list -f '{{.Dir}}' ./...)
	@echo "> test files checked successfully"

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

run:
	@make build
	$(APP_EXECUTABLE_DIR)/app

# DOCKER

# VAULT
start-vault:
	@echo "> starting vault..."
	docker-compose up -d vault
	@echo "> vault started successfully"
	docker logs auth-service-vault

stop-vault:
	@echo "> stopping vault..."
	docker-compose stop vault
	@echo "> vault stopped successfully"

restart-vault:
	@echo "> restarting vault..."
	@make stop-vault
	@make start-vault
	@echo "> vault restarted successfully"

# REDIS
start-redis:
	@echo "> starting redis..."
	docker-compose up -d redis
	@echo "> redis started successfully"
	docker logs auth-service-redis

stop-redis:
	@echo "> stopping redis..."
	docker-compose stop redis
	@echo "> redis stopped successfully"

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

DB_URL=postgresql://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@$(POSTGRES_HOST):$(POSTGRES_PORT)/$(POSTGRES_DB)?sslmode=disable

migrate-up:
	migrate -path ./migration -database "$(DB_URL)" -verbose up

migrate-down:
	migrate -path ./migration -database "$(DB_URL)" -verbose down 1

.PHONY: migrate-up migrate-down add-test-user remove-test-user testdata-up testdata-down

# Тестовые данные: список имён (без .up.sql/.down.sql), порядок = порядок накатывания
TESTDATA_ENTRIES := test_user test_space test_notes test_shared_space

DB ?= local

add-test-user:
	@echo "> adding test_user..."
	docker exec -i auth-service-postgres-$(DB) psql -v ON_ERROR_STOP=1 -h localhost -U $(POSTGRES_USER) -d $(POSTGRES_DB) < $(CUR_DIR)/testdata/test_user.up.sql
	@echo "> test_user applied"

remove-test-user:
	@echo "> removing test_user..."
	docker exec -i auth-service-postgres-$(DB) psql -v ON_ERROR_STOP=1 -h localhost -U $(POSTGRES_USER) -d $(POSTGRES_DB) < $(CUR_DIR)/testdata/test_user.down.sql
	@echo "> test_user rolled back"

add-test-space:
	@echo "> adding test_space..."
	docker exec -i auth-service-postgres-$(DB) psql -v ON_ERROR_STOP=1 -h localhost -U $(POSTGRES_USER) -d $(POSTGRES_DB) < $(CUR_DIR)/testdata/test_space.up.sql
	@echo "> test_space applied"

remove-test-space:
	@echo "> removing test_space..."
	docker exec -i auth-service-postgres-$(DB) psql -v ON_ERROR_STOP=1 -h localhost -U $(POSTGRES_USER) -d $(POSTGRES_DB) < $(CUR_DIR)/testdata/test_space.down.sql
	@echo "> test_space rolled back"

add-test-notes:
	@echo "> adding test_notes..."
	docker exec -i auth-service-postgres-$(DB) psql -v ON_ERROR_STOP=1 -h localhost -U $(POSTGRES_USER) -d $(POSTGRES_DB) < $(CUR_DIR)/testdata/test_notes.up.sql
	@echo "> test_notes applied"

remove-test-notes:
	@echo "> removing test_notes..."
	docker exec -i auth-service-postgres-$(DB) psql -v ON_ERROR_STOP=1 -h localhost -U $(POSTGRES_USER) -d $(POSTGRES_DB) < $(CUR_DIR)/testdata/test_notes.down.sql
	@echo "> test_notes rolled back"

# Накатить все тестовые данные из testdata (порядок задаётся TESTDATA_ENTRIES)
testdata-up:
	@echo "> applying testdata..."
	@set -e;
	@for name in $(TESTDATA_ENTRIES); do \
		echo "> applying testdata/$$name.up.sql..."; \
		docker exec -i auth-service-postgres-$(DB) psql -v ON_ERROR_STOP=1 -h localhost -U $(POSTGRES_USER) -d $(POSTGRES_DB) < $(CUR_DIR)/testdata/$$name.up.sql; \
	done
	@echo "> testdata applied"

# Откатить все тестовые данные (обратный порядок)
testdata-down:
	@echo "> rolling back testdata..."
	@set -e;
	@for name in $$(echo $(TESTDATA_ENTRIES) | tr ' ' '\n' | tac); do \
		echo "> rolling back testdata/$$name.down.sql..."; \
		docker exec -i auth-service-postgres-$(DB) psql -v ON_ERROR_STOP=1 -h localhost -U $(POSTGRES_USER) -d $(POSTGRES_DB) < $(CUR_DIR)/testdata/$$name.down.sql; \
	done
	@echo "> testdata rolled back"

.PHONY: add-test-user remove-test-user add-test-space remove-test-space add-test-notes remove-test-notes testdata-up testdata-down

# DOCKER
-include .env
# Настройки по умолчанию, можно переопределять через env
REGISTRY ?= docker.io
IMAGE_TAG ?= latest                   # или $(shell git rev-parse --short HEAD)
FULL_IMAGE := $(REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)

.PHONY: docker-build docker-push docker-login
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

docker-push:
	docker push $(FULL_IMAGE)

# Опционально: логин, чтобы не писать руками
docker-login:
	@echo "Logging in to $(REGISTRY)..."
	docker login $(REGISTRY)

docker-tests-up:
	docker compose -f docker-compose.tests.yaml up --build --abort-on-container-exit --exit-code-from pytests

docker-tests-down:
	docker compose -f docker-compose.tests.yaml down

.PHONY: docker-tests-up docker-tests-down
