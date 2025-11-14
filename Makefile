MAKEFILE_PATH := $(abspath $(firstword $(MAKEFILE_LIST)))
CUR_DIR := $(patsubst %/,%, $(dir $(MAKEFILE_PATH)))
BUILD_DIR := $(CUR_DIR)/.build
APP_EXECUTABLE_DIR := $(BUILD_DIR)/bin

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
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
	@echo "> golangci-lint installed successfully"
	golangci-lint --version
	
	@echo "> installing staticcheck"
	go install honnef.co/go/tools/cmd/staticcheck@latest
	@echo "> staticcheck installed successfully"
	staticcheck --version
	
	@echo "> linters installed successfully"

lint:
	@echo "> linting..."
	go vet ./...
	staticcheck ./...
	golangci-lint run ./...
	@echo "> linting successfully finished"

test:
	@echo "> testing..."
	go test -cover -gcflags="-l" -race -v ./...
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
	go mod verify
	@echo "> go.mod checked successfully"

check-test-files:
	@echo "> checking test files..."
	./scripts/check-test-files.sh $$(go list -f '{{.Dir}}' ./...)
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