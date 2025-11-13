package vault

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hashicorp/vault/api"
	"github.com/sirupsen/logrus"
)

// Client - клиент для работы с Vault.
type Client struct {
	client          *api.Client
	address         string
	token           string
	insecureSkipTLS bool
	caPath          string
	clientCertPath  string
	clientKeyPath   string
}

// ClientOption - опция для настройки клиента Vault.
type ClientOption func(*Client)

// WithAddress устанавливает адрес Vault.
func WithAddress(address string) ClientOption {
	return func(vc *Client) {
		vc.address = address
	}
}

// WithToken устанавливает токен для Vault.
func WithToken(token string) ClientOption {
	return func(vc *Client) {
		vc.token = token
	}
}

// WithInsecureSkipTLS устанавливает флаг пропуска проверки TLS сертификата.
func WithInsecureSkipTLS(insecure bool) ClientOption {
	return func(vc *Client) {
		vc.insecureSkipTLS = insecure
	}
}

// WithTLSConfig устанавливает пути к TLS сертификатам.
func WithTLSConfig(caPath, clientCertPath, clientKeyPath string) ClientOption {
	return func(vc *Client) {
		vc.caPath = caPath
		vc.clientCertPath = clientCertPath
		vc.clientKeyPath = clientKeyPath
	}
}

// NewClient создает новый клиент для работы с Vault.
func NewClient(opts ...ClientOption) (*Client, error) {
	vaultClient := &Client{}

	for _, opt := range opts {
		opt(vaultClient)
	}

	if vaultClient.address == "" {
		return nil, errors.New("address is required")
	}

	if vaultClient.token == "" {
		return nil, errors.New("token is required")
	}

	if !vaultClient.insecureSkipTLS {
		if vaultClient.caPath == "" {
			return nil, errors.New("CA certificate is required")
		}
	}

	// Проверяем, что клиентский сертификат и ключ указаны вместе (либо ничего из этого)
	if (vaultClient.clientCertPath != "" && vaultClient.clientKeyPath == "") || (vaultClient.clientCertPath == "" && vaultClient.clientKeyPath != "") {
		return nil, errors.New("client certificate and key must be provided together")
	}

	return vaultClient, nil
}

// validateAndResolvePath проверяет существование файла и преобразует путь в абсолютный.
func validateAndResolvePath(path, fileType string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("vault: error resolving %s path: %w", fileType, err)
	}

	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return "", fmt.Errorf("vault: %s file not found: %s", fileType, absPath)
	}

	return absPath, nil
}

// configureCA настраивает CA сертификат в TLS конфигурации.
func (vc *Client) configureCA(tlsConfig *api.TLSConfig) error {
	if vc.caPath == "" {
		return nil
	}

	caPath, err := validateAndResolvePath(vc.caPath, "CA certificate")
	if err != nil {
		return err
	}

	tlsConfig.CAPath = caPath
	logrus.WithField("ca_path", caPath).Debug("using CA certificate for server verification")

	return nil
}

// configureClientCertificates настраивает клиентский сертификат и ключ в TLS конфигурации.
func (vc *Client) configureClientCertificates(tlsConfig *api.TLSConfig) error {
	hasCert := vc.clientCertPath != ""
	hasKey := vc.clientKeyPath != ""

	if !hasCert && !hasKey {
		return nil
	}

	if hasCert != hasKey {
		return errors.New("vault: client certificate and key must be provided together")
	}

	clientCertPath, err := validateAndResolvePath(vc.clientCertPath, "client certificate")
	if err != nil {
		return err
	}

	clientKeyPath, err := validateAndResolvePath(vc.clientKeyPath, "client key")
	if err != nil {
		return err
	}

	tlsConfig.ClientCert = clientCertPath
	tlsConfig.ClientKey = clientKeyPath
	logrus.WithFields(logrus.Fields{
		"client_cert": clientCertPath,
		"client_key":  clientKeyPath,
	}).Debug("using client certificate and key")

	return nil
}

// shouldConfigureTLS проверяет, нужно ли настраивать TLS.
func (vc *Client) shouldConfigureTLS() bool {
	return vc.insecureSkipTLS || vc.caPath != "" || (vc.clientCertPath != "" && vc.clientKeyPath != "")
}

// configureTLS настраивает TLS конфигурацию для клиента Vault.
func (vc *Client) configureTLS(config *api.Config) error {
	tlsConfig := &api.TLSConfig{
		Insecure: vc.insecureSkipTLS,
	}

	if err := vc.configureCA(tlsConfig); err != nil {
		return err
	}

	if err := vc.configureClientCertificates(tlsConfig); err != nil {
		return err
	}

	if !vc.shouldConfigureTLS() {
		return nil
	}

	if err := config.ConfigureTLS(tlsConfig); err != nil {
		return fmt.Errorf("vault: error configuring TLS: %w", err)
	}

	return nil
}

// createAPIClient создает и настраивает API клиент Vault.
func (vc *Client) createAPIClient() (*api.Client, error) {
	config := api.DefaultConfig()
	config.Address = vc.address

	if err := vc.configureTLS(config); err != nil {
		return nil, err
	}

	client, err := api.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("vault: error creating client: %w", err)
	}

	client.SetToken(vc.token)

	return client, nil
}

// verifyConnection проверяет соединение с Vault через Health API.
func (vc *Client) verifyConnection(client *api.Client) error {
	logrus.WithFields(logrus.Fields{
		"address":           vc.address,
		"insecure_skip_tls": vc.insecureSkipTLS,
	}).Info("trying to connect to vault...")

	health, err := client.Sys().Health()
	if err != nil {
		return fmt.Errorf("vault: failed to connect to vault at %s: %w", vc.address, err)
	}

	logrus.WithFields(logrus.Fields{
		"address": vc.address,
		"version": health.Version,
		"sealed":  health.Sealed,
	}).Info("connected to vault")

	return nil
}

// Connect подключается к Vault и проверяет соединение.
// Делает запрос к Health API для проверки соединения.
func (vc *Client) Connect() error {
	client, err := vc.createAPIClient()
	if err != nil {
		return err
	}

	if err := vc.verifyConnection(client); err != nil {
		return err
	}

	vc.client = client

	return nil
}

// Stop останавливает клиент Vault.
// Vault API клиент использует стандартный http.Client, который автоматически
// управляет соединениями. При завершении работы приложения все соединения
// будут закрыты автоматически. Здесь мы просто обнуляем ссылку на клиент.
func (vc *Client) Stop(ctx context.Context) error {
	if vc.client == nil {
		return nil
	}

	// Обнуляем клиент. HTTP клиент внутри api.Client автоматически
	// закроет все idle соединения при завершении работы приложения.
	vc.client = nil

	logrus.Info("vault client stopped")

	// Проверяем контекст на отмену
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	return nil
}
