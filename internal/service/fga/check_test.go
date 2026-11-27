package fga

import (
	"context"
	"io"
	"testing"
	"time"

	"auth-service/internal/model"
	"auth-service/internal/service/fga/mocks"

	"github.com/golang/mock/gomock"
	openfga "github.com/openfga/go-sdk"
	sdk "github.com/openfga/go-sdk/client"
	"github.com/stretchr/testify/require"
)

func TestCheckRelation(t *testing.T) {
	t.Parallel()

	response := func(allowed bool) *sdk.ClientCheckResponse {
		return &sdk.ClientCheckResponse{CheckResponse: openfga.CheckResponse{Allowed: openfga.PtrBool(allowed)}}
	}
	wantResponse := func(wantAllowed bool, consistency openfga.ConsistencyPreference) func(*testing.T, *Client, *checkRequestStub, bool) {
		return func(t *testing.T, c *Client, stub *checkRequestStub, allowed bool) {
			t.Helper()
			require.Equal(t, wantAllowed, allowed)
			require.Equal(t, "pinned", c.ModelID())
			require.Equal(t, "pinned", *stub.GetOptions().AuthorizationModelId)
			require.Equal(t, sdk.ClientCheckRequest{User: "user:u", Relation: "can_view", Object: "note:n"}, *stub.GetBody())
			require.Equal(t, consistency, *stub.GetOptions().Consistency)
		}
	}

	wantFailure := func(t *testing.T, _ *Client, _ *checkRequestStub, allowed bool) {
		t.Helper()
		require.False(t, allowed)
	}

	for _, tt := range []struct {
		name        string
		consistency model.Consistency
		response    *sdk.ClientCheckResponse
		err         error
		expect      func(*testing.T, *mocks.MockSdkClient, *checkRequestStub)
		wantErr     require.ErrorAssertionFunc
		want        func(*testing.T, *Client, *checkRequestStub, bool)
	}{
		{"allow", model.HigherConsistency, response(true), nil, expectRelationCheck, require.NoError, wantResponse(true, "HIGHER_CONSISTENCY")},
		{"deny", model.MinimizeLatency, response(false), nil, expectRelationCheck, require.NoError, wantResponse(false, "MINIMIZE_LATENCY")},
		{"transport error", model.HigherConsistency, nil, io.ErrUnexpectedEOF, expectRelationCheck, func(t require.TestingT, err error, _ ...any) { require.ErrorIs(t, err, io.ErrUnexpectedEOF) }, wantFailure},
		{"deadline at SDK boundary", model.HigherConsistency, nil, nil, expectCheckTimeout, func(t require.TestingT, err error, _ ...any) { require.ErrorIs(t, err, context.DeadlineExceeded) }, wantFailure},
		{"missing allowed result", model.HigherConsistency, &sdk.ClientCheckResponse{}, nil, expectRelationCheck, requireModelError("missing allowed result"), wantFailure},
		{"empty", model.HigherConsistency, nil, nil, expectRelationCheck, requireModelError("empty response"), wantFailure},
		{"invalid", model.Consistency("bad"), nil, nil, func(*testing.T, *mocks.MockSdkClient, *checkRequestStub) {}, requireModelError("unsupported consistency"), wantFailure},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			api := mocks.NewMockSdkClient(gomock.NewController(t))
			c := &Client{fgaClient: api, modelID: "pinned", requestTimeout: time.Second}

			stub := &checkRequestStub{err: tt.err, response: tt.response}
			tt.expect(t, api, stub)

			allowed, err := c.CheckRelation(t.Context(), "user:u", "can_view", "note:n", tt.consistency)
			tt.wantErr(t, err)

			tt.want(t, c, stub, allowed)
		})
	}
}

func expectRelationCheck(t *testing.T, api *mocks.MockSdkClient, stub *checkRequestStub) {
	t.Helper()

	api.EXPECT().Check(gomock.Any()).DoAndReturn(func(ctx context.Context) sdk.SdkClientCheckRequestInterface {
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		require.Positive(t, time.Until(deadline))
		require.LessOrEqual(t, time.Until(deadline), time.Second)

		return stub
	})
}

func expectCheckTimeout(t *testing.T, api *mocks.MockSdkClient, stub *checkRequestStub) {
	t.Helper()
	api.EXPECT().Check(gomock.Any()).DoAndReturn(func(ctx context.Context) sdk.SdkClientCheckRequestInterface {
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		require.LessOrEqual(t, time.Until(deadline), time.Second)
		<-ctx.Done()
		stub.err = ctx.Err()

		return stub
	})
}
