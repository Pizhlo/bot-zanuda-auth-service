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
	Redis  Redis  `yaml:"redis" validate:"required"`
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

// RedisType - тип подключения к Redis: single - один узел, cluster - кластер.
type RedisType string

const (
	// RedisTypeSingle - один узел.
	RedisTypeSingle RedisType = "single"
	// RedisTypeCluster - кластер.
	RedisTypeCluster RedisType = "cluster"
)

// Redis - конфигурация Redis.
type Redis struct {
	Type RedisType `yaml:"type" validate:"required,oneof=single cluster"`
	// single
	Host string `yaml:"host" validate:"omitempty,hostname"`
	Port int    `yaml:"port" validate:"omitempty,min=1024,max=65535"`
	// cluster
	Addrs []string `yaml:"addrs" validate:"omitempty,dive,hostname_port"`
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

	if err := cfg.validateRedisConfig(); err != nil {
		return nil, fmt.Errorf("config: error validate redis: %w", err)
	}

	return cfg, nil
}

func (cfg *Config) validateRedisConfig() error {
	switch cfg.Redis.Type {
	case RedisTypeSingle:
		return validateRedisSingleConfig(&cfg.Redis)
	case RedisTypeCluster:
		return validateRedisClusterConfig(&cfg.Redis)
	}

	// нет default, т.к. валидируется в validate.Struct
	return nil
}

func validateRedisSingleConfig(cfg *Redis) error {
	if cfg.Host == "" || cfg.Port == 0 {
		return fmt.Errorf("config: host and port are required for single redis")
	}

	if len(cfg.Addrs) > 0 {
		return fmt.Errorf("config: addrs are not allowed for single redis")
	}

	return nil
}

func validateRedisClusterConfig(cfg *Redis) error {
	if len(cfg.Addrs) == 0 {
		return fmt.Errorf("config: addrs are required for cluster redis")
	}

	if cfg.Host != "" || cfg.Port != 0 {
		return fmt.Errorf("config: host and port are not allowed for cluster redis")
	}

	return nil
}
