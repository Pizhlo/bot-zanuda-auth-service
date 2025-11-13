package config

import (
	"fmt"
	"os"
	"time"

	"github.com/go-playground/validator/v10"
	"gopkg.in/yaml.v2"
)

// Config - конфигурация всего сервиса.
type Config struct {
	LogLevel string `yaml:"log_level" validate:"required,oneof=debug info warn error"`

	Server Server `yaml:"server" validate:"required"`
	Vault  Vault  `yaml:"vault" validate:"required"`
}

// Server - конфигурация сервера.
type Server struct {
	Port            int           `yaml:"port" validate:"required,min=1024,max=65535"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout" validate:"required,min=1ms"`
	SwaggerHost     string        `yaml:"swagger_host" validate:"omitempty,hostname_port"` // Опциональный host для swagger (например, "localhost:8080" или "api.example.com")
}

// Vault - конфигурация Vault.
type Vault struct {
	Address         string `yaml:"address" validate:"required,url"`
	Token           string `yaml:"token" validate:"required"`
	InsecureSkipTLS bool   `yaml:"insecure_skip_tls"` // Пропускать проверку TLS сертификата (только для разработки)
	CAPath          string `yaml:"ca_path"`           // Путь к CA сертификату (опционально)
	ClientCertPath  string `yaml:"client_cert_path"`  // Путь к клиентскому сертификату (опционально)
	ClientKeyPath   string `yaml:"client_key_path"`   // Путь к клиентскому ключу (опционально)
}

// LoadConfig загружает конфигурацию.
func LoadConfig(path string) (*Config, error) {
	cfg := &Config{}

	// Читаем YAML файл
	yamlFile, err := os.ReadFile(path) //nolint:gosec // заведена задача на исправление BZ-100
	if err != nil {
		return nil, fmt.Errorf("config: error read file: %w", err)
	}

	// Парсим YAML
	if err := yaml.Unmarshal(yamlFile, cfg); err != nil {
		return nil, fmt.Errorf("config: error unmarshal: %w", err)
	}

	validate := validator.New()

	if err := validate.Struct(cfg); err != nil {
		return nil, fmt.Errorf("config: error validate: %w", err)
	}

	return cfg, nil
}
