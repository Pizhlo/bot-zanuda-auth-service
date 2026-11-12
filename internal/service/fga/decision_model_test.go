package fga

import (
	fgamodel "auth-service/fga"
	"auth-service/internal/testfga"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPinnedDecisionModel(t *testing.T) {
	t.Parallel()
	api := testfga.New(t, fgamodel.Strict)
	id, err := api.GetAuthorizationModelId()
	require.NoError(t, err)

	c := &Client{fgaClient: api, modelID: id}
	require.NoError(t, c.EnsureDecisionModel(t.Context()))
	transitionID := testfga.SetModel(t, api, fgamodel.Transition)
	// Публикация другой latest не меняет закреплённую модель check.
	require.NoError(t, c.EnsureDecisionModel(t.Context()))
	c.modelID = transitionID
	require.ErrorContains(t, c.EnsureDecisionModel(t.Context()), "does not match")
}
