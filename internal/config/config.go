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
}

// Server - конфигурация сервера.
type Server struct {
	Port            int           `yaml:"port" validate:"required,min=1024,max=65535"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout" validate:"required,min=1ms"`
	SwaggerHost     string        `yaml:"swagger_host" validate:"omitempty,hostname_port"` // Опциональный host для swagger (например, "localhost:8080" или "api.example.com")
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
