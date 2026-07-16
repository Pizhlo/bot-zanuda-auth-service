package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoginRequest_IsEmpty(t *testing.T) {
	t.Parallel()

	r := LoginRequest{GrantType: ClientCredentialsGrantType, ClientID: "test", ClientSecret: "test", Scope: BotScope}
	require.False(t, r.IsEmpty())

	r = LoginRequest{GrantType: "", ClientID: "", ClientSecret: "", Scope: ""}
	require.True(t, r.IsEmpty())
}
