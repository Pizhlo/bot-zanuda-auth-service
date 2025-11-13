package vault

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/hashicorp/vault/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//nolint:funlen // длинный тест - это ок
func TestNewClient(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		options []ClientOption
		want    *Client
		wantErr require.ErrorAssertionFunc
	}{
		{
			name: "positive case: address and token with insecureSkipTLS",
			options: []ClientOption{
				WithAddress("https://localhost:8200"),
				WithToken("vault-token"),
				WithInsecureSkipTLS(true),
			},
			want: &Client{
				address:         "https://localhost:8200",
				token:           "vault-token",
				insecureSkipTLS: true,
			},
			wantErr: require.NoError,
		},
		{
			name: "positive case: with CA certificate only",
			options: []ClientOption{
				WithAddress("https://localhost:8200"),
				WithToken("vault-token"),
				WithTLSConfig("/path/to/ca.pem", "", ""),
			},
			want: &Client{
				address:        "https://localhost:8200",
				token:          "vault-token",
				caPath:         "/path/to/ca.pem",
				clientCertPath: "",
				clientKeyPath:  "",
			},
			wantErr: require.NoError,
		},
		{
			name: "positive case: with full TLS config",
			options: []ClientOption{
				WithAddress("https://localhost:8200"),
				WithToken("vault-token"),
				WithTLSConfig("/path/to/ca.pem", "/path/to/cert.pem", "/path/to/key.pem"),
			},
			want: &Client{
				address:        "https://localhost:8200",
				token:          "vault-token",
				caPath:         "/path/to/ca.pem",
				clientCertPath: "/path/to/cert.pem",
				clientKeyPath:  "/path/to/key.pem",
			},
			wantErr: require.NoError,
		},
		{
			name: "positive case: with insecureSkipTLS and TLS config",
			options: []ClientOption{
				WithAddress("https://localhost:8200"),
				WithToken("vault-token"),
				WithInsecureSkipTLS(true),
				WithTLSConfig("/path/to/ca.pem", "/path/to/cert.pem", "/path/to/key.pem"),
			},
			want: &Client{
				address:         "https://localhost:8200",
				token:           "vault-token",
				insecureSkipTLS: true,
				caPath:          "/path/to/ca.pem",
				clientCertPath:  "/path/to/cert.pem",
				clientKeyPath:   "/path/to/key.pem",
			},
			wantErr: require.NoError,
		},
		{
			name:    "error case: address is required",
			options: []ClientOption{WithToken("vault-token")},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "address is required")
			},
		},
		{
			name:    "error case: token is required",
			options: []ClientOption{WithAddress("https://localhost:8200")},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "token is required")
			},
		},
		{
			name: "error case: CA certificate is required",
			options: []ClientOption{
				WithAddress("https://localhost:8200"),
				WithToken("vault-token"),
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "CA certificate is required")
			},
		},
		{
			name: "error case: client certificate without key",
			options: []ClientOption{
				WithAddress("https://localhost:8200"),
				WithToken("vault-token"),
				WithTLSConfig("/path/to/ca.pem", "/path/to/cert.pem", ""),
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "client certificate and key must be provided together")
			},
		},
		{
			name: "error case: client key without certificate",
			options: []ClientOption{
				WithAddress("https://localhost:8200"),
				WithToken("vault-token"),
				WithTLSConfig("/path/to/ca.pem", "", "/path/to/key.pem"),
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "client certificate and key must be provided together")
			},
		},
		{
			name: "error case: client cert and key without CA when insecureSkipTLS is false",
			options: []ClientOption{
				WithAddress("https://localhost:8200"),
				WithToken("vault-token"),
				WithTLSConfig("", "/path/to/cert.pem", "/path/to/key.pem"),
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "CA certificate is required")
			},
		},
		{
			name: "positive case: client cert and key without CA when insecureSkipTLS is true",
			options: []ClientOption{
				WithAddress("https://localhost:8200"),
				WithToken("vault-token"),
				WithInsecureSkipTLS(true),
				WithTLSConfig("", "/path/to/cert.pem", "/path/to/key.pem"),
			},
			want: &Client{
				address:         "https://localhost:8200",
				token:           "vault-token",
				insecureSkipTLS: true,
				caPath:          "",
				clientCertPath:  "/path/to/cert.pem",
				clientKeyPath:   "/path/to/key.pem",
			},
			wantErr: require.NoError,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client, err := NewClient(tt.options...)
			tt.wantErr(t, err)

			if tt.want != nil {
				require.NotNil(t, client)
				assert.Equal(t, tt.want, client)
			}
		})
	}
}

func TestStop(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		ctx       func() context.Context
		client    *Client
		checkWant func(t *testing.T, client *Client)
		wantErr   require.ErrorAssertionFunc
	}{
		{
			name:   "positive case",
			ctx:    func() context.Context { return t.Context() },
			client: &Client{client: &api.Client{}},
			checkWant: func(t *testing.T, client *Client) {
				t.Helper()

				assert.Nil(t, client.client)
			},
			wantErr: require.NoError,
		},
		{
			name:   "client is nil",
			ctx:    func() context.Context { return t.Context() },
			client: &Client{client: nil},
			checkWant: func(t *testing.T, client *Client) {
				t.Helper()

				assert.Nil(t, client.client)
			},
			wantErr: require.NoError,
		},
		{
			name:   "error case: context is done",
			ctx:    func() context.Context { ctx, cancel := context.WithCancel(t.Context()); cancel(); return ctx },
			client: &Client{client: &api.Client{}},
			checkWant: func(t *testing.T, client *Client) {
				t.Helper()

				assert.Nil(t, client.client)
			},
			wantErr: require.Error,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.client.Stop(tt.ctx())
			tt.wantErr(t, err)

			tt.checkWant(t, tt.client)
		})
	}
}

//nolint:funlen // длинный тест - это ок
func TestValidateAndResolvePath(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		fileType string
		setup    func(t *testing.T) string
		wantErr  require.ErrorAssertionFunc
		check    func(t *testing.T, result string)
	}{
		{
			name:     "positive case: existing file",
			fileType: "test certificate",
			setup: func(t *testing.T) string {
				t.Helper()

				tmpDir := t.TempDir()
				tmpFile := filepath.Join(tmpDir, "test-cert.pem")

				file, err := os.Create(tmpFile) //nolint:gosec // тестовый файл
				require.NoError(t, err)

				err = file.Close()
				require.NoError(t, err)

				return tmpFile
			},
			wantErr: require.NoError,
			check: func(t *testing.T, result string) {
				t.Helper()
				assert.True(t, filepath.IsAbs(result))
				assert.FileExists(t, result)
			},
		},
		{
			name:     "positive case: relative path",
			fileType: "test certificate",
			setup: func(t *testing.T) string {
				t.Helper()

				tmpDir := t.TempDir()
				tmpFile := filepath.Join(tmpDir, "test-cert.pem")

				file, err := os.Create(tmpFile) //nolint:gosec // тестовый файл
				require.NoError(t, err)

				err = file.Close()
				require.NoError(t, err)

				// Меняем рабочую директорию на временную
				oldDir, err := os.Getwd()
				require.NoError(t, err)

				err = os.Chdir(tmpDir)
				require.NoError(t, err)

				t.Cleanup(func() {
					err := os.Chdir(oldDir)
					require.NoError(t, err)
				})

				return filepath.Base(tmpFile)
			},
			wantErr: require.NoError,
			check: func(t *testing.T, result string) {
				t.Helper()
				assert.True(t, filepath.IsAbs(result))
				assert.FileExists(t, result)
			},
		},
		{
			name:     "error case: file not found",
			fileType: "test certificate",
			setup: func(t *testing.T) string {
				t.Helper()
				return "/nonexistent/path/to/file.pem"
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "test certificate file not found")
			},
			check: func(t *testing.T, result string) {
				t.Helper()
				assert.Empty(t, result)
			},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			testPath := tt.setup(t)

			result, err := validateAndResolvePath(testPath, tt.fileType)
			tt.wantErr(t, err)

			if tt.check != nil {
				tt.check(t, result)
			}
		})
	}
}

//nolint:funlen // длинный тест - это ок
func TestConfigureTLS(t *testing.T) {
	t.Parallel()

	// Получаем путь к testdata относительно текущего файла
	_, filename, _, _ := runtime.Caller(0) //nolint:dogsled // используемся для тестирования
	testdataDir := filepath.Join(filepath.Dir(filename), "testdata")

	testCases := []struct {
		name           string
		setupClient    func(t *testing.T) *Client
		wantErr        require.ErrorAssertionFunc
		checkTLSConfig func(t *testing.T, config *api.Config)
	}{
		{
			name: "positive case: no TLS config",
			setupClient: func(t *testing.T) *Client {
				t.Helper()

				return &Client{
					insecureSkipTLS: false,
					caPath:          "",
					clientCertPath:  "",
					clientKeyPath:   "",
				}
			},
			wantErr: require.NoError,
		},
		{
			name: "positive case: insecure skip TLS",
			setupClient: func(t *testing.T) *Client {
				t.Helper()

				return &Client{
					insecureSkipTLS: true,
					caPath:          "",
					clientCertPath:  "",
					clientKeyPath:   "",
				}
			},
			wantErr: require.NoError,
		},
		{
			name: "positive case: CA certificate only",
			setupClient: func(t *testing.T) *Client {
				t.Helper()

				caPath := filepath.Join(testdataDir, "ca.pem")

				return &Client{
					insecureSkipTLS: false,
					caPath:          caPath,
					clientCertPath:  "",
					clientKeyPath:   "",
				}
			},
			wantErr: require.NoError,
		},
		{
			name: "positive case: client certificate and key",
			setupClient: func(t *testing.T) *Client {
				t.Helper()

				certPath := filepath.Join(testdataDir, "client-cert.pem")
				keyPath := filepath.Join(testdataDir, "client-key.pem")

				return &Client{
					insecureSkipTLS: false,
					caPath:          "",
					clientCertPath:  certPath,
					clientKeyPath:   keyPath,
				}
			},
			wantErr: require.NoError,
		},
		{
			name: "positive case: full TLS config",
			setupClient: func(t *testing.T) *Client {
				t.Helper()

				caPath := filepath.Join(testdataDir, "ca.pem")
				certPath := filepath.Join(testdataDir, "client-cert.pem")
				keyPath := filepath.Join(testdataDir, "client-key.pem")

				return &Client{
					insecureSkipTLS: false,
					caPath:          caPath,
					clientCertPath:  certPath,
					clientKeyPath:   keyPath,
				}
			},
			wantErr: require.NoError,
		},
		{
			name: "positive case: insecure skip TLS with certificates",
			setupClient: func(t *testing.T) *Client {
				t.Helper()

				caPath := filepath.Join(testdataDir, "ca.pem")

				return &Client{
					insecureSkipTLS: true,
					caPath:          caPath,
					clientCertPath:  "",
					clientKeyPath:   "",
				}
			},
			wantErr: require.NoError,
		},
		{
			name: "error case: CA certificate file not found",
			setupClient: func(t *testing.T) *Client {
				t.Helper()

				return &Client{
					insecureSkipTLS: false,
					caPath:          "/nonexistent/ca.pem",
					clientCertPath:  "",
					clientKeyPath:   "",
				}
			},
			wantErr: require.Error,
		},
		{
			name: "error case: client certificate file not found",
			setupClient: func(t *testing.T) *Client {
				t.Helper()

				return &Client{
					insecureSkipTLS: false,
					caPath:          "",
					clientCertPath:  "/nonexistent/client-cert.pem",
					clientKeyPath:   "/nonexistent/client-key.pem",
				}
			},
			wantErr: require.Error,
		},
		{
			name: "error case: client key file not found",
			setupClient: func(t *testing.T) *Client {
				t.Helper()

				certPath := filepath.Join(testdataDir, "client-cert.pem")

				return &Client{
					insecureSkipTLS: false,
					caPath:          "",
					clientCertPath:  certPath,
					clientKeyPath:   "/nonexistent/client-key.pem",
				}
			},
			wantErr: require.Error,
		},
		{
			name: "error case: multiple files not found",
			setupClient: func(t *testing.T) *Client {
				t.Helper()

				return &Client{
					insecureSkipTLS: false,
					caPath:          "/nonexistent/ca.pem",
					clientCertPath:  "/nonexistent/client-cert.pem",
					clientKeyPath:   "",
				}
			},
			wantErr: require.Error,
		},
		{
			name: "error case: client certificate without key",
			setupClient: func(t *testing.T) *Client {
				t.Helper()

				certPath := filepath.Join(testdataDir, "client-cert.pem")

				return &Client{
					insecureSkipTLS: false,
					caPath:          "",
					clientCertPath:  certPath,
					clientKeyPath:   "",
				}
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "client certificate and key must be provided together")
			},
		},
		{
			name: "error case: client key without certificate",
			setupClient: func(t *testing.T) *Client {
				t.Helper()

				keyPath := filepath.Join(testdataDir, "client-key.pem")

				return &Client{
					insecureSkipTLS: false,
					caPath:          "",
					clientCertPath:  "",
					clientKeyPath:   keyPath,
				}
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "client certificate and key must be provided together")
			},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := tt.setupClient(t)
			config := api.DefaultConfig()
			err := client.configureTLS(config)

			tt.wantErr(t, err)

			if err != nil {
				assert.Contains(t, err.Error(), "vault:")
			}

			if tt.checkTLSConfig != nil {
				tt.checkTLSConfig(t, config)
			}
		})
	}
}
