package auth

import (
	"auth-service/internal/service/auth/mocks"
	"auth-service/pkg/audit"
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//nolint:funlen // длинный тест - ничего страшного; похожие тест-кейсы
func TestCheckToken(t *testing.T) {
	t.Parallel()

	type test struct {
		name         string
		token        string
		headerPrefix string
		cause        string
		want         jwt.MapClaims
		wantErr      require.ErrorAssertionFunc
		check        func(*testing.T, jwt.MapClaims, *jwt.Token)
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

	anotherMethodToken, err := jwt.NewWithClaims(jwt.SigningMethodHS384, tokenClaims{
		Scope: "bot",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "test",
			Subject:   "bot",
			Audience:  []string{internalAPIAudience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(tokenDuration)),
		},
	}).SignedString([]byte("secret"))
	require.NoError(t, err)

	unsignedToken, err := jwt.NewWithClaims(jwt.SigningMethodNone, tokenClaims{
		Scope: "bot",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "test",
			Subject:   "bot",
			Audience:  []string{internalAPIAudience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(tokenDuration)),
		},
	}).SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)

	noIssuerToken := generateTestToken(t, tokenClaims{
		Scope: "bot",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "bot",
			Audience:  []string{internalAPIAudience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(tokenDuration)),
		},
	}, []byte("secret"))

	noAudienceToken := generateTestToken(t, tokenClaims{
		Scope: "bot",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "test",
			Subject:   "bot",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(tokenDuration)),
		},
	}, []byte("secret"))

	noExpiredToken := generateTestToken(t, tokenClaims{
		Scope: "bot",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:   "test",
			Subject:  "bot",
			Audience: []string{internalAPIAudience},
			IssuedAt: jwt.NewNumericDate(now),
		},
	}, []byte("secret"))

	tokenWithoutIat := generateTestToken(t, tokenClaims{
		Scope: "bot",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "test",
			Subject:   "bot",
			Audience:  []string{internalAPIAudience},
			ExpiresAt: jwt.NewNumericDate(now.Add(tokenDuration)),
		},
	}, []byte("secret"))

	checkValid := func(t *testing.T, claims jwt.MapClaims, token *jwt.Token) {
		t.Helper()
		require.NotNil(t, token)
		require.True(t, token.Valid)
		require.Equal(t, "HS256", token.Method.Alg())
		require.Equal(t, claims, token.Claims)
	}
	checkMissing := func(t *testing.T, _ jwt.MapClaims, token *jwt.Token) { t.Helper(); require.Nil(t, token) }
	wantError := func(message string) require.ErrorAssertionFunc {
		return func(t require.TestingT, err error, _ ...any) { require.EqualError(t, err, message) }
	}

	tests := []test{
		{
			name:         "positive case (uppercase)",
			headerPrefix: "Bearer ",
			token:        token,
			want: jwt.MapClaims{
				"scope": "bot",
				"iss":   "test",
				"sub":   "bot",
				"aud":   []any{internalAPIAudience},
				"iat":   float64(jwt.NewNumericDate(now).Unix()),
				"exp":   float64(jwt.NewNumericDate(now.Add(tokenDuration)).Unix()),
			},
			wantErr: require.NoError,
			check:   checkValid,
		},
		{
			name:         "positive case (lowercase)",
			headerPrefix: "bearer ",
			token:        token,
			want: jwt.MapClaims{
				"scope": "bot",
				"iss":   "test",
				"sub":   "bot",
				"aud":   []any{internalAPIAudience},
				"iat":   float64(jwt.NewNumericDate(now).Unix()),
				"exp":   float64(jwt.NewNumericDate(now.Add(tokenDuration)).Unix()),
			},
			wantErr: require.NoError,
			check:   checkValid,
		},
		{
			name:         "error case: invalid token",
			cause:        "token is malformed",
			headerPrefix: "Bearer ",
			token:        "malformed-fixture-token",
			wantErr:      wantError("invalid token"),
			check:        checkMissing,
		},
		{
			name:         "error case: expired",
			cause:        "token has invalid claims: token is expired",
			headerPrefix: "Bearer ",
			token:        expiredToken,
			wantErr:      wantError("invalid token"),
			check:        checkMissing,
		},
		{
			name:  "error case: no 'Bearer' prefix",
			cause: "invalid token: no prefix Bearer",
			token: token,
			want: jwt.MapClaims{
				"scope": "bot",
				"iss":   "test",
				"sub":   "bot",
				"aud":   []any{internalAPIAudience},
				"iat":   float64(jwt.NewNumericDate(now).Unix()),
				"exp":   float64(jwt.NewNumericDate(now.Add(tokenDuration)).Unix()),
			},
			wantErr: wantError("invalid token: no prefix Bearer"),
			check:   checkMissing,
		},
		{
			name:         "error case: empty token",
			cause:        "invalid token: empty bearer token",
			headerPrefix: "Bearer ",
			wantErr:      wantError("invalid token: empty bearer token"),
			check:        checkMissing,
		},
		{
			name:         "error case: another method token",
			cause:        "token signature is invalid",
			headerPrefix: "Bearer ",
			token:        anotherMethodToken,
			wantErr:      wantError("invalid token"),
			check:        checkMissing,
		},
		{
			name:         "error case: unsigned token",
			cause:        "token signature is invalid",
			headerPrefix: "Bearer ",
			token:        unsignedToken,
			wantErr:      wantError("invalid token"),
			check:        checkMissing,
		},
		{
			name:         "error case: no issuer",
			cause:        "token has invalid claims: token is missing required claim: iss claim is required",
			headerPrefix: "Bearer ",
			token:        noIssuerToken,
			wantErr:      wantError("invalid token"),
			check:        checkMissing,
		},
		{
			name:         "error case: no audience",
			cause:        "token has invalid claims: token is missing required claim: aud claim is required",
			headerPrefix: "Bearer ",
			token:        noAudienceToken,
			wantErr:      wantError("invalid token"),
			check:        checkMissing,
		},
		{
			name:         "error case: no expired",
			cause:        "token has invalid claims: token is missing required claim: exp claim is required",
			headerPrefix: "Bearer ",
			token:        noExpiredToken,
			wantErr:      wantError("invalid token"),
			check:        checkMissing,
		},
		{
			name:         "error case: no iat",
			cause:        "token is missing required claim: iat claim is required",
			headerPrefix: "Bearer ",
			token:        tokenWithoutIat,
			wantErr:      wantError("invalid token"),
			check:        checkMissing,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			auth := createTestAuthService(t, []byte("secret"), nil)

			var output bytes.Buffer

			ctx, attempt, err := audit.Begin(t.Context(), audit.NewAuditor(audit.WithOutput(&output)), audit.OperationFilterNotes, audit.StageAuthenticate.String())
			require.NoError(t, err)

			token, err := auth.CheckToken(ctx, tt.headerPrefix+tt.token)
			tt.wantErr(t, err)

			attempt.Finish(ctx, &err)

			tt.check(t, tt.want, token)
			require.Equal(t, tt.cause != "", strings.Contains(output.String(), "cause="))
			require.Contains(t, output.String(), tt.cause)
			require.Equal(t, tt.cause != "", strings.Contains(output.String(), "level=warn"))
			// Пустой Bearer не содержит JWT; проверяем отсутствие самого заголовка.
			require.NotContains(t, output.String(), cmp.Or(tt.token, tt.headerPrefix))
		})
	}
}

func TestTokenAuditCause(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		err, want error
	}{
		{"signed claims", errors.Join(jwt.ErrTokenInvalidClaims, jwt.ErrTokenExpired), errors.Join(jwt.ErrTokenInvalidClaims, jwt.ErrTokenExpired)},
		{"signature", fmt.Errorf("%w: untrusted alg", jwt.ErrTokenSignatureInvalid), jwt.ErrTokenSignatureInvalid},
		{"malformed", fmt.Errorf("%w: untrusted input", jwt.ErrTokenMalformed), jwt.ErrTokenMalformed},
		{"unverifiable", fmt.Errorf("%w: untrusted key", jwt.ErrTokenUnverifiable), jwt.ErrTokenUnverifiable},
		{"unknown", errors.New("private error"), ErrInvalidToken},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.EqualError(t, tokenAuditCause(tc.err), tc.want.Error())
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

//nolint:funlen // длинный тест - ничего страшного
func TestParseToken(t *testing.T) {
	t.Parallel()

	now := time.Now()
	validToken := generateTestToken(t, tokenClaims{
		Scope: "bot",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "test",
			Subject:   "bot",
			Audience:  []string{internalAPIAudience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
		},
	}, []byte("secret"))

	invalidIssuerToken := generateTestToken(t, tokenClaims{
		Scope: "bot",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "another-issuer",
			Subject:   "bot",
			Audience:  []string{internalAPIAudience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
		},
	}, []byte("secret"))

	noAudienceToken := generateTestToken(t, tokenClaims{
		Scope: "bot",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "test",
			Subject:   "bot",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
		},
	}, []byte("secret"))

	anotherMethodToken, err := jwt.NewWithClaims(jwt.SigningMethodHS384, tokenClaims{
		Scope: "bot",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "test",
			Subject:   "bot",
			Audience:  []string{internalAPIAudience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
		},
	}).SignedString([]byte("secret"))
	require.NoError(t, err)

	tests := []struct {
		name        string
		tokenString string
		wantErr     require.ErrorAssertionFunc
		check       func(t *testing.T, token *jwt.Token, claims jwt.MapClaims)
	}{
		{
			name:        "valid token",
			tokenString: validToken,
			wantErr:     require.NoError,
			check: func(t *testing.T, token *jwt.Token, claims jwt.MapClaims) {
				t.Helper()
				require.NotNil(t, token)
				require.True(t, token.Valid)
				require.Equal(t, "bot", claims["scope"])
			},
		},
		{
			name:        "invalid issuer",
			tokenString: invalidIssuerToken,
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "token has invalid claims")
			},
			check: func(t *testing.T, token *jwt.Token, claims jwt.MapClaims) {
				t.Helper()
				require.Nil(t, token)
				require.Empty(t, claims)
			},
		},
		{
			name:        "missing audience",
			tokenString: noAudienceToken,
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "token has invalid claims")
			},
			check: func(t *testing.T, token *jwt.Token, claims jwt.MapClaims) {
				t.Helper()
				require.Nil(t, token)
				require.Empty(t, claims)
			},
		},
		{
			name:        "invalid signing method",
			tokenString: anotherMethodToken,
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "token signature is invalid")
			},
			check: func(t *testing.T, token *jwt.Token, claims jwt.MapClaims) {
				t.Helper()
				require.Nil(t, token)
				require.Empty(t, claims)
			},
		},
		{
			name:        "malformed token",
			tokenString: "malformed.token",
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "token is malformed")
			},
			check: func(t *testing.T, token *jwt.Token, claims jwt.MapClaims) {
				t.Helper()
				require.Nil(t, token)
				require.Empty(t, claims)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svc := createTestAuthService(t, []byte("secret"), nil)
			token, claims, err := svc.parseToken(tt.tokenString)

			tt.wantErr(t, err)
			tt.check(t, token, claims)
		})
	}
}

func TestValidateIAT(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		claims  jwt.MapClaims
		wantErr require.ErrorAssertionFunc
	}{
		{
			name: "valid iat",
			claims: jwt.MapClaims{
				"iat": float64(time.Now().Unix()),
			},
			wantErr: require.NoError,
		},
		{
			name:   "missing iat",
			claims: jwt.MapClaims{},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorIs(t, err, jwt.ErrTokenRequiredClaimMissing)
			},
		},
		{
			name: "invalid iat type",
			claims: jwt.MapClaims{
				"iat": "not-a-number",
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorIs(t, err, jwt.ErrInvalidType)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateIAT(tt.claims)
			tt.wantErr(t, err)
		})
	}
}

type mockMocks struct {
	mockVaultClient *mocks.MockvaultClient
	mockStorage     *mocks.Mockstorage
}

func createTestAuthService(t *testing.T, secretKey []byte, setupMocks func(t *testing.T, m *mockMocks)) *Service {
	t.Helper()

	updateKey := 1 * time.Minute

	ctrl := gomock.NewController(t)

	mockVaultClient := mocks.NewMockvaultClient(ctrl)
	mockStorage := mocks.NewMockstorage(ctrl)

	m := &mockMocks{
		mockVaultClient: mockVaultClient,
		mockStorage:     mockStorage,
	}

	if setupMocks != nil {
		setupMocks(t, m)
	}

	auth, err := New(
		WithSecretKey(secretKey),
		WithUpdateKeyInterval(updateKey),
		WithVaultClient(mockVaultClient),
		WithStorage(mockStorage),
		WithIssuer("test"),
		WithTokenDuration(time.Second),
	)
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
