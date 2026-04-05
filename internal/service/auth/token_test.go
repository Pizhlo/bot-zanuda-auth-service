package auth

import (
	"auth-service/internal/service/auth/mocks"
	serviceinternal "auth-service/internal/service/internal"
	"auth-service/pkg/audit"
	"auth-service/pkg/audit/testaudit"
	"context"
	"reflect"
	"testing"
	"time"
	"unsafe"

	"github.com/golang-jwt/jwt/v5"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//nolint:funlen,dupl // длинный тест - ничего страшного; похожие тест-кейсы
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

	tests := []test{
		{
			name:  "positive case (uppercase)",
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
			name:  "positive case (lowercase)",
			token: "bearer " + token,
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
				assert.EqualError(t, err, "invalid token")
				assert.Nil(t, token)
			},
		},
		{
			name:  "error case: expired",
			token: "Bearer " + expiredToken,
			check: func(err error, tt test, token *jwt.Token) {
				require.Error(t, err)
				assert.EqualError(t, err, "invalid token")
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
		{
			name:  "error case: empty token",
			token: "Bearer ",
			check: func(err error, tt test, token *jwt.Token) {
				require.Error(t, err)
				assert.EqualError(t, err, "invalid token: empty bearer token")
				assert.Nil(t, token)
			},
		},
		{
			name:  "error case: another method token",
			token: "Bearer " + anotherMethodToken,
			check: func(err error, tt test, token *jwt.Token) {
				require.Error(t, err)
				assert.EqualError(t, err, "invalid token")
				assert.Nil(t, token)
			},
		},
		{
			name:  "error case: unsigned token",
			token: "Bearer " + unsignedToken,
			check: func(err error, tt test, token *jwt.Token) {
				require.Error(t, err)
				assert.EqualError(t, err, "invalid token")
				assert.Nil(t, token)
			},
		},
		{
			name:  "error case: no issuer",
			token: "Bearer " + noIssuerToken,
			check: func(err error, tt test, token *jwt.Token) {
				require.Error(t, err)
				assert.EqualError(t, err, "invalid token")
				assert.Nil(t, token)
			},
		},
		{
			name:  "error case: no audience",
			token: "Bearer " + noAudienceToken,
			check: func(err error, tt test, token *jwt.Token) {
				require.Error(t, err)
				assert.EqualError(t, err, "invalid token")
				assert.Nil(t, token)
			},
		},
		{
			name:  "error case: no expired",
			token: "Bearer " + noExpiredToken,
			check: func(err error, tt test, token *jwt.Token) {
				require.Error(t, err)
				assert.EqualError(t, err, "invalid token")
				assert.Nil(t, token)
			},
		},
		{
			name:  "error case: no iat",
			token: "Bearer " + tokenWithoutIat,
			check: func(err error, tt test, token *jwt.Token) {
				require.Error(t, err)
				assert.EqualError(t, err, "invalid token")
				assert.Nil(t, token)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			auth := createTestAuthService(t, []byte("secret"), nil)
			token, err := auth.CheckToken(t.Context(), tt.token)
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
				require.ErrorIs(t, err, ErrInvalidToken)
			},
		},
		{
			name: "invalid iat type",
			claims: jwt.MapClaims{
				"iat": "not-a-number",
			},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorIs(t, err, ErrInvalidToken)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			event := testaudit.NewAuditor(t).Create(t.Context())

			err := validateIAT(tt.claims, event)
			tt.wantErr(t, err)
		})
	}
}

func TestTokenValidationFailedHook(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctx = serviceinternal.WithServiceName(ctx)
	ctx = serviceinternal.WithMessage(ctx, "failed to validate token")
	ctx = serviceinternal.WithLevel(ctx, audit.ErrLevelWarn)
	ctx = serviceinternal.WithErrorCode(ctx, audit.ErrCodeTokenInvalid)
	ctx = serviceinternal.WithMessageCtx(ctx, audit.EventContext{"ip_address": "127.0.0.1"})
	ctx = serviceinternal.WithUserID(ctx, "user-1")
	ctx = serviceinternal.WithOperation(ctx, "auth-service.check_token")
	ctx = serviceinternal.WithKind(ctx, audit.KindValidation)

	var captured audit.Stash

	auditor := audit.NewAuditor(
		audit.WithHook(TokenValidationFailedHook),
		audit.WithHook(func(ctx context.Context, stash audit.Stash) audit.Stash {
			captured = stash
			return stash
		}),
	)

	_ = auditor.Create(ctx)
	values := stashValues(captured)

	require.Contains(t, values, "auth-service")
	require.Contains(t, values, "failed to validate token")
	require.Contains(t, values, audit.ErrLevelWarn)
	require.Contains(t, values, audit.ErrCodeTokenInvalid)
	require.Contains(t, values, "user-1")
	require.Contains(t, values, "auth-service.check_token")
	require.Contains(t, values, audit.KindValidation)
	require.True(t, containsByDeepEqual(values, audit.EventContext{"ip_address": "127.0.0.1"}))
}

func stashValues(stash audit.Stash) []any {
	v := reflect.ValueOf(&stash).Elem()

	fields := v.FieldByName("fields")
	if !fields.IsValid() || fields.IsNil() {
		return nil
	}

	// Поле неэкспортируемое, поэтому читаем его через unsafe для теста.
	fields = reflect.NewAt(fields.Type(), unsafe.Pointer(fields.UnsafeAddr())).Elem()

	values := make([]any, 0, fields.Len())

	iter := fields.MapRange()
	for iter.Next() {
		fieldValue := iter.Value()
		fieldIface := fieldValue.Interface()

		valueField := reflect.ValueOf(fieldIface).FieldByName("Value")
		if valueField.IsValid() && valueField.CanInterface() {
			values = append(values, valueField.Interface())
		}
	}

	return values
}

func containsByDeepEqual(values []any, want any) bool {
	for _, v := range values {
		if reflect.DeepEqual(v, want) {
			return true
		}
	}

	return false
}

type mockMocks struct {
	mockVaultClient *mocks.MockvaultClient
	mockStorage     *mocks.Mockstorage
	mockAuditor     auditor
}

func createTestAuthService(t *testing.T, secretKey []byte, setupMocks func(t *testing.T, m *mockMocks)) *Service {
	t.Helper()

	updateKey := 1 * time.Minute

	ctrl := gomock.NewController(t)

	mockVaultClient := mocks.NewMockvaultClient(ctrl)
	mockStorage := mocks.NewMockstorage(ctrl)
	mockAuditor := testaudit.NewAuditor(t)

	m := &mockMocks{
		mockVaultClient: mockVaultClient,
		mockStorage:     mockStorage,
		mockAuditor:     mockAuditor,
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
		WithAuditor(mockAuditor),
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
