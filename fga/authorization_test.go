package fgamodel_test

import (
	"testing"

	fgamodel "auth-service/fga"
	"auth-service/internal/testfga"
	openfga "github.com/openfga/go-sdk"
	sdk "github.com/openfga/go-sdk/client"
	"github.com/stretchr/testify/require"
)

func TestAuthorizationMatrix(t *testing.T) {
	t.Parallel()
	api := testfga.New(t, fgamodel.Strict)
	seedMatrix(t, api)

	for _, space := range []string{"personal", "shared", "missing", "conflict"} {
		for _, role := range []string{"owner", "admin", "editor", "viewer", "record-owner", "stranger"} {
			for _, kind := range []string{"space", "note", "reminder"} {
				checkMatrixResource(t, api, space, role, kind)
			}
		}
	}
}

func expected(space, role, kind, relation string) bool {
	if space == "missing" || space == "conflict" {
		return false
	}

	if space == "personal" {
		return role == "owner" && relation != "can_manage_members" && (kind != "space" || relation != "can_delete")
	}

	return expectedShared(role, kind, relation)
}

func expectedShared(role, kind, relation string) bool {
	if role == "owner" {
		return true
	}

	if kind != "space" && role == "record-owner" {
		return true
	}

	switch relation {
	case "can_view":
		return role == "viewer" || role == "editor" || role == "admin"
	case "can_create_note", "can_create_reminder", "can_update", "can_stop":
		return role == "editor" || role == "admin"
	case "can_manage_members":
		return role == "admin"
	default:
		return false
	}
}

func seedMatrix(t *testing.T, api *sdk.OpenFgaClient) {
	t.Helper()

	tuples := []sdk.ClientTupleKey{}

	for _, space := range []string{"personal", "shared", "missing", "conflict"} {
		object := "space:" + space

		tuples = append(tuples, sdk.ClientTupleKey{User: "user:owner", Relation: "owner", Object: object})
		for _, role := range []string{"viewer", "editor", "admin"} {
			tuples = append(tuples, sdk.ClientTupleKey{User: "user:" + role, Relation: role, Object: object})
		}

		if space != "missing" {
			marker := space
			if space == "conflict" {
				marker = "personal"
			}

			tuples = append(tuples, sdk.ClientTupleKey{User: "user:*", Relation: marker, Object: object})
		}

		if space == "conflict" {
			tuples = append(tuples, sdk.ClientTupleKey{User: "user:*", Relation: "shared", Object: object})
		}

		for _, kind := range []string{"note", "reminder"} {
			child := kind + ":" + space
			tuples = append(tuples, sdk.ClientTupleKey{User: object, Relation: "space", Object: child}, sdk.ClientTupleKey{User: "user:record-owner", Relation: "owner", Object: child})
		}
	}

	testfga.Write(t, api, tuples...)
}

func checkMatrixResource(t *testing.T, api *sdk.OpenFgaClient, space, role, kind string) {
	t.Helper()

	relations := []string{"can_view", "can_update", "can_delete"}
	if kind == "space" {
		relations = []string{"can_view", "can_manage_space", "can_delete", "can_manage_members", "can_create_note", "can_create_reminder"}
	}

	if kind == "reminder" {
		relations = append(relations, "can_stop")
	}

	for _, relation := range relations {
		t.Run(space+"/"+role+"/"+kind+"/"+relation, func(t *testing.T) {
			t.Parallel()

			preference := openfga.CONSISTENCYPREFERENCE_HIGHER_CONSISTENCY
			resp, err := api.Check(t.Context()).Body(sdk.ClientCheckRequest{User: "user:" + role, Relation: relation, Object: kind + ":" + space}).Options(sdk.ClientCheckOptions{Consistency: &preference}).Execute()
			require.NoError(t, err)
			require.Equal(t, expected(space, role, kind, relation), resp.GetAllowed())
		})
	}
}
