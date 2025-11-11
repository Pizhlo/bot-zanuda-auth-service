package main

import (
	"auth-service/docs"
	handlerV0 "auth-service/internal/api/v0"
	"auth-service/internal/config"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitHandlerV0(t *testing.T) {
	t.Parallel()

	buildInfo := &BuildInfo{
		Version:   "1.0.0",
		BuildDate: "2021-01-01",
		GitCommit: "1234567890",
	}

	hv0 := initHandlerV0(buildInfo)
	require.NotNil(t, hv0)

	assert.Equal(t, handlerV0.Version0, hv0.Version())
}

func TestInitServer(t *testing.T) {
	t.Parallel()

	buildInfo := &BuildInfo{
		Version:   "1.0.0",
		BuildDate: "2021-01-01",
		GitCommit: "1234567890",
	}

	handlerV0 := initHandlerV0(buildInfo)
	require.NotNil(t, handlerV0)

	server := initServer(handlerV0, config.Server{
		Port:            8080,
		ShutdownTimeout: 10 * time.Second,
	})
	require.NotNil(t, server)
}

func TestUpdateSwaggerHost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  config.Server
		want string
	}{
		{
			name: "positive case",
			cfg:  config.Server{Port: 8080, ShutdownTimeout: 10 * time.Second, SwaggerHost: "localhost:1234"},
			want: "localhost:1234",
		},
		{
			name: "negative case",
			cfg:  config.Server{Port: 8080, ShutdownTimeout: 10 * time.Second, SwaggerHost: ""},
			want: "localhost:8080",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			updateSwaggerHost(tt.cfg)
			require.Equal(t, tt.want, docs.SwaggerInfo.Host)
		})
	}
}
