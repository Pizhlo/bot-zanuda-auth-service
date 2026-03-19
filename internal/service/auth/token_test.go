package auth

import (
	"auth-service/internal/service/auth/mocks"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//nolint:funlen // длинный тест - ничего страшного
func TestCheckToken(t *testing.T) {
	t.Parallel()

	type test struct {
		name  string
		token string
		want  jwt.MapClaims
		check func(err error, tt test, token *jwt.Token)
	}

	now := time.Now()
	tokenDuration := 1 * time.Hour

	// payload: {"user_id":123,"exp":1875018533}
	token := generateTestToken(t, tokenClaims{
		Scope: "bot",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "test",
			Subject:   "bot",
			Audience:  []string{internalAPIAudience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(tokenDuration)),
		},
	}, []byte("secret"))

	// payload: {"user_id":123,"exp":1717252133}
	expiredToken := generateTestToken(t, tokenClaims{
		Scope: "bot",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "test",
			Subject:   "bot",
			Audience:  []string{internalAPIAudience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(-1 * time.Second)),
		},
	}, []byte("secret"))

	tests := []test{
		{
			name:  "positive case",
			token: "Bearer " + token,
			want: jwt.MapClaims{
				"scope": "bot",
				"iss":   "test",
				"sub":   "bot",
				"aud":   []any{internalAPIAudience},
				"iat":   float64(jwt.NewNumericDate(now).Unix()),
				"exp":   float64(jwt.NewNumericDate(now.Add(tokenDuration)).Unix()),
			},
			check: func(err error, tt test, token *jwt.Token) {
				require.NoError(t, err)
				require.NotNil(t, token)
				assert.True(t, token.Valid)
				assert.Equal(t, "HS256", token.Method.Alg())
				assert.Equal(t, tt.want, token.Claims)
			},
		},
		{
			name:  "error case: invalid token",
			token: "Bearer invalid",
			check: func(err error, tt test, token *jwt.Token) {
				require.Error(t, err)
				assert.EqualError(t, err, "invalid token: token is malformed: token contains an invalid number of segments")
				assert.Nil(t, token)
			},
		},
		{
			name:  "error case: expired",
			token: "Bearer " + expiredToken,
			check: func(err error, tt test, token *jwt.Token) {
				require.Error(t, err)
				assert.EqualError(t, err, "invalid token: token has invalid claims: token is expired")
				assert.Nil(t, token)
			},
		},
		{
			name:  "error case: no 'Bearer' prefix",
			token: token,
			want: jwt.MapClaims{
				"scope": "bot",
				"iss":   "test",
				"sub":   "bot",
				"aud":   []any{internalAPIAudience},
				"iat":   float64(jwt.NewNumericDate(now).Unix()),
				"exp":   float64(jwt.NewNumericDate(now.Add(tokenDuration)).Unix()),
			},
			check: func(err error, tt test, token *jwt.Token) {
				require.Error(t, err)
				assert.EqualError(t, err, "invalid token: no prefix Bearer")
				assert.Nil(t, token)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			auth := createTestAuthService(t, []byte("secret"), nil)
			token, err := auth.CheckToken(tt.token)
			tt.check(err, tt, token)
		})
	}
}

func TestGetPayload(t *testing.T) {
	t.Parallel()

	type test struct {
		name       string
		token      *jwt.Token
		wantClaims jwt.MapClaims
		wantOk     bool
	}

	validClaims := jwt.MapClaims{
		"user_id": float64(123),
		"expired": float64(1875018533),
	}

	tests := []test{
		{
			name: "positive case",
			token: &jwt.Token{
				Claims: jwt.MapClaims{
					"user_id": float64(123),
					"expired": float64(1875018533),
				},
			},
			wantClaims: validClaims,
			wantOk:     true,
		},
		{
			name: "error case: invalid claims type",
			token: &jwt.Token{
				Claims: &jwt.RegisteredClaims{}, // используем указатель на RegisteredClaims
			},
			wantClaims: nil,
			wantOk:     false,
		},
		{
			name:       "error case: nil token",
			token:      nil,
			wantClaims: nil,
			wantOk:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			auth := createTestAuthService(t, []byte("secret"), nil)
			claims, ok := auth.GetPayload(tt.token)
			assert.Equal(t, tt.wantOk, ok)
			assert.Equal(t, tt.wantClaims, claims)
		})
	}
}

func TestGenerateToken(t *testing.T) {
	t.Parallel()

	auth := createTestAuthService(t, []byte("secret"), nil)
	token, err := auth.generateToken(tokenClaims{
		Scope: "bot",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "test",
			Subject:   "bot",
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Second)),
			Audience:  []string{internalAPIAudience},
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, token)

	tokenJWT, err := jwt.Parse(token, func(token *jwt.Token) (interface{}, error) {
		return []byte("secret"), nil
	})

	require.NoError(t, err)
	require.NotNil(t, tokenJWT)
	require.True(t, tokenJWT.Valid)

	claims, ok := auth.GetPayload(tokenJWT)
	require.True(t, ok)
	require.Equal(t, "bot", claims["scope"])
	require.Equal(t, "test", claims["iss"])
	require.Equal(t, "bot", claims["sub"])
	require.Equal(t, []any{internalAPIAudience}, claims["aud"])
	require.NotEmpty(t, claims["iat"])
	require.NotEmpty(t, claims["exp"])
}

func createTestAuthService(t *testing.T, secretKey []byte, setupMocks func(t *testing.T, mockVaultClient *mocks.MockvaultClient, mockStorage *mocks.Mockstorage)) *Service {
	t.Helper()

	updateKey := 1 * time.Minute

	ctrl := gomock.NewController(t)

	mockVaultClient := mocks.NewMockvaultClient(ctrl)
	mockStorage := mocks.NewMockstorage(ctrl)

	if setupMocks != nil {
		setupMocks(t, mockVaultClient, mockStorage)
	}

	auth, err := New(WithSecretKey(secretKey), WithUpdateKeyInterval(updateKey), WithVaultClient(mockVaultClient), WithStorage(mockStorage), WithIssuer("test"), WithTokenDuration(time.Second))
	require.NoError(t, err)

	return auth
}

func generateTestToken(t *testing.T, claims tokenClaims, secretKey []byte) string {
	t.Helper()

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	signed, err := token.SignedString(secretKey)
	require.NoError(t, err)

	return signed
}
