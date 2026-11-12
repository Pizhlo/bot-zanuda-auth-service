package fga

import (
	"context"
	"testing"

	"auth-service/internal/model"
	"auth-service/internal/service/fga/mocks"
	"github.com/golang/mock/gomock"
	openfga "github.com/openfga/go-sdk"
	sdk "github.com/openfga/go-sdk/client"
	"github.com/stretchr/testify/require"
)

func TestCheckRelation(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name        string
		consistency model.Consistency
		allowed     bool
		err         error
		response    bool
	}{
		{"allow", model.HigherConsistency, true, nil, true}, {"deny", model.MinimizeLatency, false, nil, true},
		{"failure", model.HigherConsistency, false, context.DeadlineExceeded, false}, {"empty", model.HigherConsistency, false, nil, false},
		{"invalid", model.Consistency("bad"), false, nil, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			api := mocks.NewMockSdkClient(gomock.NewController(t))
			c := &Client{fgaClient: api, modelID: "pinned"}

			stub := &checkRequestStub{err: tt.err}
			if tt.response {
				stub.response = &sdk.ClientCheckResponse{CheckResponse: openfga.CheckResponse{Allowed: openfga.PtrBool(tt.allowed)}}
			}

			if tt.name != "invalid" {
				api.EXPECT().Check(gomock.Any()).Return(stub)
			}

			allowed, err := c.CheckRelation(t.Context(), "user:u", "can_view", "note:n", tt.consistency)
			if !tt.response {
				require.Error(t, err)
				require.False(t, allowed)

				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.allowed, allowed)
			require.Equal(t, "pinned", c.ModelID())
			require.Equal(t, "pinned", *stub.GetOptions().AuthorizationModelId)
			require.Equal(t, sdk.ClientCheckRequest{User: "user:u", Relation: "can_view", Object: "note:n"}, *stub.GetBody())
			require.Equal(t, openfga.ConsistencyPreference(map[model.Consistency]string{model.HigherConsistency: "HIGHER_CONSISTENCY", model.MinimizeLatency: "MINIMIZE_LATENCY"}[tt.consistency]), *stub.GetOptions().Consistency)
		})
	}
}
