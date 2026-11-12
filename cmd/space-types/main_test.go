package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	fgamodel "auth-service/fga"
	"auth-service/internal/service/fga"
	"auth-service/internal/service/spacemigration"
	"auth-service/internal/testfga"
	"github.com/google/uuid"
	sdk "github.com/openfga/go-sdk/client"
	"github.com/stretchr/testify/require"
)

func TestRunRejectsInvalidArgumentsAndExport(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{nil, {"-unknown"}, {"positional"}, {"-api-url", "http://localhost", "-store-id", "store", "-model-id", "model", "-input", "/does-not-exist"}} {
		t.Run("invalid", func(t *testing.T) {
			t.Parallel()

			var output bytes.Buffer
			require.Error(t, run(t.Context(), args, &output))
			require.Empty(t, output.String())
		})
	}

	path := filepath.Join(t.TempDir(), "export.json")
	require.NoError(t, os.WriteFile(path, []byte("not json"), 0600))
	_, err := readExport(path)
	require.ErrorContains(t, err, "decode trusted export")
}

func TestRunDryRunAndApply(t *testing.T) {
	t.Parallel()
	api := testfga.New(t, fgamodel.Transition)
	id, owner := uuid.New(), uuid.New()
	testfga.Write(t, api, sdk.ClientTupleKey{Object: "space:" + id.String(), Relation: "owner", User: "user:" + owner.String()})
	data, err := json.Marshal([]spacemigration.Record{{SpaceID: id, OwnerID: owner, SpaceType: "personal"}})
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "export.json")
	require.NoError(t, os.WriteFile(path, data, 0600))

	store, err := api.GetStoreId()
	require.NoError(t, err)
	model, err := api.GetAuthorizationModelId()
	require.NoError(t, err)

	args := []string{"-api-url", api.GetConfig().ApiUrl, "-store-id", store, "-model-id", model, "-input", path}

	var dry bytes.Buffer
	require.NoError(t, execute(args, &dry))

	var plan spacemigration.Plan
	require.NoError(t, json.Unmarshal(dry.Bytes(), &plan))
	require.Len(t, plan.Writes, 1)
	before, err := fga.ReadTuples(t.Context(), api, "")
	require.NoError(t, err)
	require.Len(t, before, 1)

	var applied bytes.Buffer
	require.NoError(t, run(t.Context(), append([]string{"-apply"}, args...), &applied))
	require.JSONEq(t, dry.String(), applied.String())
	after, err := fga.ReadTuples(t.Context(), api, "")
	require.NoError(t, err)
	require.Len(t, after, 2)

	var replay bytes.Buffer
	require.NoError(t, run(t.Context(), args, &replay))
	require.NoError(t, json.Unmarshal(replay.Bytes(), &plan))
	require.Empty(t, plan.Writes)
}
