package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"gopkg.in/yaml.v2"
)

// Config - конфигурация всего сервиса.
type Config struct {
	LogLevel string `yaml:"log_level" validate:"required,oneof=debug info warn error"`

	Server   Server   `yaml:"server" validate:"required"`
	Vault    Vault    `yaml:"vault" validate:"required"`
	Redis    Redis    `yaml:"redis" validate:"required"`
	Postgres Postgres `yaml:"postgres" validate:"required"`
	Auth     Auth     `yaml:"auth" validate:"required"`
	Policy   Policy   `yaml:"policy" validate:"required"`
	RabbitMQ RabbitMQ `yaml:"rabbitmq"`
}

// Auth - данные для работы Auth-сервиса.
type Auth struct {
	SecretKey         string        `yaml:"secret_key" validate:"required"`            // секретный ключ для генерации токена.
	UpdateKeyInterval time.Duration `yaml:"update_key_interval" validate:"required"`   // интервал для обновления апи-ключа в Vault.
	Issuer            string        `yaml:"issuer" validate:"required"`                // issuer для генерации токена.
	TokenDuration     time.Duration `yaml:"token_duration" validate:"required,min=1"`  // duration access токена.
	UserCacheTTL      time.Duration `yaml:"user_cache_ttl" validate:"required,min=1s"` // duration кэша пользователей.
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
	InsecureSkipTLS bool   `yaml:"insecure_skip_tls" validate:"omitempty"` // Пропускать проверку TLS сертификата (только для разработки)
	CAPath          string `yaml:"ca_path" validate:"omitempty"`           // Путь к CA сертификату (опционально)
	ClientCertPath  string `yaml:"client_cert_path" validate:"omitempty"`  // Путь к клиентскому сертификату (опционально)
	ClientKeyPath   string `yaml:"client_key_path" validate:"omitempty"`   // Путь к клиентскому ключу (опционально)
	SecretsPath     string `yaml:"secrets_path" validate:"required"`       // где хранится bot API key в Vault
}

// RabbitMQ - конфигурация RabbitMQ.
type RabbitMQ struct {
	URL            string        `yaml:"url"`
	ConnectTimeout time.Duration `yaml:"connect_timeout"`
	PublishTimeout time.Duration `yaml:"publish_timeout"`
	MaxRetries     int           `yaml:"max_retries"`
	RetryBackoff   time.Duration `yaml:"retry_backoff"`
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

// Postgres - конфигурация PostgreSQL.
type Postgres struct {
	Host          string        `yaml:"host" validate:"required"`
	Port          int           `yaml:"port" validate:"required,min=1024,max=65535"`
	User          string        `yaml:"user" validate:"required"`
	Password      string        `yaml:"password" validate:"required"`
	DBName        string        `yaml:"db_name" validate:"required"`
	InsertTimeout time.Duration `yaml:"insert_timeout" validate:"required,min=1"`
	ReadTimeout   time.Duration `yaml:"read_timeout" validate:"required,min=1"`
}

// Policy - конфигурация политики.
type Policy struct {
	// Config - конфигурация модели для Casbin.
	Config string `yaml:"config" validate:"required"`
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

func validateRabbitMQConfig(cfg *RabbitMQ) error {
	if cfg.URL == "" {
		return fmt.Errorf("config: rabbitmq url is not set")
	}

	if !strings.HasPrefix(cfg.URL, "amqp://") && !strings.HasPrefix(cfg.URL, "amqps://") {
		return fmt.Errorf("config: rabbitmq url must start with amqp:// or amqps://")
	}

	u, err := url.Parse(cfg.URL)
	if err != nil {
		return fmt.Errorf("config: rabbitmq url is not valid: %w", err)
	}

	if u.Host == "" {
		return fmt.Errorf("config: rabbitmq url host is not set")
	}

	if cfg.ConnectTimeout <= 0 {
		return fmt.Errorf("config: rabbitmq connect timeout is not set")
	}

	if cfg.PublishTimeout <= 0 {
		return fmt.Errorf("config: rabbitmq publish timeout is not set")
	}

	if cfg.MaxRetries <= 0 {
		return fmt.Errorf("config: rabbitmq max retries is not set")
	}

	if cfg.RetryBackoff <= 0 {
		return fmt.Errorf("config: rabbitmq retry backoff is not set")
	}

	return nil
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
