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

	// payload: {"user_id":123,"exp":1875018533}
	token := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyX2lkIjoxMjMsImV4cCI6MTg3NTAxODUzM30.UumRMX7Y9tbgaflpAgNyzcy7BopB821isQ2M5BmSR3Y"

	// payload: {"user_id":123,"exp":1717252133}
	expiredToken := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyX2lkIjoxMjMsImV4cCI6MTcxNzI1MjEzM30.qY_w-M4vQoUJTYwV8GmvEyVlsLus892YsuJHMXGsld8"

	tests := []test{
		{
			name:  "positive case",
			token: "Bearer " + token,
			want: jwt.MapClaims{
				"user_id": float64(123),
				"exp":     float64(1875018533),
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
				"user_id": float64(123),
				"exp":     float64(1875018533),
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

			auth := createTestAuthService(t, []byte("secret"))
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

			auth := createTestAuthService(t, []byte("secret"))
			claims, ok := auth.GetPayload(tt.token)
			assert.Equal(t, tt.wantOk, ok)
			assert.Equal(t, tt.wantClaims, claims)
		})
	}
}

func createTestAuthService(t *testing.T, secretKey []byte) *service {
	t.Helper()

	updateKey := 1 * time.Minute

	ctrl := gomock.NewController(t)

	mockVaultClient := mocks.NewMockvaultClient(ctrl)

	auth, err := New(WithSecretKey(secretKey), WithUpdateKeyInterval(updateKey), WithVaultClient(mockVaultClient))
	require.NoError(t, err)

	return auth
}
