package model_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"auth-service/internal/model"
	"auth-service/internal/service/enforcer"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestDocumentedDecisionExamples(t *testing.T) {
	t.Parallel()

	for _, result := range []string{"allow", "deny"} {
		t.Run(result, func(t *testing.T) {
			t.Parallel()

			requestData, err := os.ReadFile(filepath.Join("..", "..", "docs", "examples", "decisions", "request-"+result+".json")) //nolint:gosec // Имена из закрытой таблицы allow/deny, не из внешнего ввода.
			require.NoError(t, err)
			req, err := model.DecodeDecision(requestData)
			require.NoError(t, err)
			responseData, err := os.ReadFile(filepath.Join("..", "..", "docs", "examples", "decisions", "response-"+result+".json")) //nolint:gosec // Имена из закрытой таблицы allow/deny, не из внешнего ввода.
			require.NoError(t, err)

			var resp model.CheckDecisionResponse
			require.NoError(t, json.Unmarshal(responseData, &resp))
			require.Equal(t, req.RequestID, resp.RequestID)
			require.NotEqual(t, uuid.Nil, resp.DecisionID)
			require.Equal(t, "note:"+req.Resource.ID.String(), resp.Meta.Resource)
			require.Equal(t, result, resp.Result)
			require.Equal(t, result == "allow", resp.Allow)

			policy, err := enforcer.NewFieldPolicy()
			require.NoError(t, err)
			forbidden, err := policy.CheckFields(string(req.Resource.Type), string(req.Action.Name), req.Action.Attributes.FieldMask)
			require.NoError(t, err)
			require.Equal(t, len(forbidden) == 0, resp.Allow)
			require.Equal(t, policy.Version(), resp.Meta.PolicyVersion)
		})
	}
}
