package postgres

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"testing"
	"time"

	"auth-service/internal/model"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// decisionConnector передаёт SQL драйверу в памяти, сохраняя стандартное преобразование аргументов database/sql.
type decisionConnector struct {
	driver.Connector
	conn *decisionConn
}

func (c decisionConnector) Connect(context.Context) (driver.Conn, error) { return c.conn, nil }

type decisionConn struct {
	driver.Conn
	query string
	args  []driver.NamedValue
	err   error
}

func (c *decisionConn) Close() error { return nil }
func (c *decisionConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.query = query
	c.args = args

	return driver.RowsAffected(1), c.err
}

func TestSaveDecision(t *testing.T) {
	t.Parallel()

	request, subject, decisionID, spaceID, noteID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	object := "note:" + noteID.String()

	personal := model.DecisionRecord{
		DecisionID: decisionID, RequestID: request, SubjectID: subject, ClientID: "bot",
		Action:   model.DecisionAction{Name: model.ActionNameCreate},
		Resource: model.DecisionResource{Type: model.ResourceTypeSpace, ID: spaceID, Attributes: model.DecisionResourceAttributes{SpaceType: new(model.SpaceKindPersonal)}},
		Result:   "deny", Reasons: []model.DecisionReason{{Code: "personal_space_already_exists", Message: "system space"}},
		PolicyVersion: "policy", AuthorizationModelID: "model",
	}
	note := model.DecisionRecord{
		DecisionID: decisionID, RequestID: request, SubjectID: subject, ClientID: "bot",
		Action:        model.DecisionAction{Name: model.ActionNameUpdate, Attributes: model.DecisionActionAttributes{FieldMask: []string{"title"}}},
		Resource:      model.DecisionResource{Type: model.ResourceTypeNote, ID: noteID, Attributes: model.DecisionResourceAttributes{SpaceID: &spaceID}},
		CheckedObject: &object, CheckedRelation: new("can_edit"), Result: "allow", Reasons: []model.DecisionReason{},
		PolicyVersion: "policy", AuthorizationModelID: "model",
	}
	failed := errors.New("insert failed")

	for _, tc := range []struct {
		name    string
		record  model.DecisionRecord
		dbErr   error
		wantErr require.ErrorAssertionFunc
		want    func(*testing.T, *decisionConn)
	}{
		{"create space without parent or checked relation", personal, nil, require.NoError, wantInsert(decisionID.String(), request.String(), "bot", subject.String(), "create", "{}", "space", spaceID.String(), nil, nil, nil, "deny", `[{"code":"personal_space_already_exists","message":"system space"}]`, "policy", "model", "personal")},
		{"update note records parent, relation and field mask", note, nil, require.NoError, wantInsert(decisionID.String(), request.String(), "bot", subject.String(), "update", `{"field_mask":["title"]}`, "note", noteID.String(), spaceID.String(), object, "can_edit", "allow", "[]", "policy", "model", nil)},
		{"database write failed", personal, failed, func(t require.TestingT, err error, _ ...any) {
			require.ErrorIs(t, err, failed)
			require.ErrorContains(t, err, "insert decision")
		}, func(t *testing.T, conn *decisionConn) {
			t.Helper()
			require.NotEmpty(t, conn.query)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			conn := &decisionConn{err: tc.dbErr}
			db := sql.OpenDB(decisionConnector{conn: conn})

			t.Cleanup(func() { require.NoError(t, db.Close()) })

			repo := Repo{db: db, insertTimeout: time.Second}

			err := repo.SaveDecision(t.Context(), tc.record)
			tc.wantErr(t, err)

			tc.want(t, conn)
		})
	}
}

func wantInsert(values ...any) func(*testing.T, *decisionConn) {
	return func(t *testing.T, conn *decisionConn) {
		t.Helper()
		require.Contains(t, conn.query, "INSERT INTO auth.decisions")

		got := make([]any, 0, len(conn.args))
		for _, arg := range conn.args {
			got = append(got, arg.Value)
		}

		require.Equal(t, values, got)
	}
}
