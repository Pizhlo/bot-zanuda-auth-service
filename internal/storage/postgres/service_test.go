package politics

import (
	"database/sql"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//nolint:funlen // это тест
func TestNew(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		opts    []RepoOption
		want    *Repo
		wantErr require.ErrorAssertionFunc
	}{
		{
			name: "positive case",
			opts: []RepoOption{
				WithAddr("postgres://user:password@localhost:5432/dbname?sslmode=disable"),
				WithInsertTimeout(1),
				WithReadTimeout(2),
			},
			want: &Repo{
				addr:          "postgres://user:password@localhost:5432/dbname?sslmode=disable",
				insertTimeout: 1,
				readTimeout:   2,
			},
			wantErr: require.NoError,
		},
		{
			name: "negative case: addr is empty",
			opts: []RepoOption{
				WithInsertTimeout(1),
				WithReadTimeout(2),
			},
			want: nil,
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "addr is required")
			},
		},
		{
			name: "negative case: insert timeout is 0",
			opts: []RepoOption{
				WithAddr("postgres://user:password@localhost:5432/dbname?sslmode=disable"),
				WithReadTimeout(2),
			},
			want: nil,
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "insert timeout is required")
			},
		},
		{
			name: "negative case: read timeout is 0",
			opts: []RepoOption{
				WithAddr("postgres://user:password@localhost:5432/dbname?sslmode=disable"),
				WithInsertTimeout(1),
			},
			want: nil,
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "read timeout is required")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := New(t.Context(), tt.opts...)
			tt.wantErr(t, err)

			if tt.want != nil {
				require.NotNil(t, got)

				assert.Equal(t, tt.want.addr, got.addr)
				assert.Equal(t, tt.want.insertTimeout, got.insertTimeout)
				assert.Equal(t, tt.want.readTimeout, got.readTimeout)

				assert.NotNil(t, got.db)
				assert.Equal(t, reflect.TypeOf(&sql.DB{}), reflect.TypeOf(got.db))

				// Закрываем соединение с базой данных для предотвращения утечек ресурсов
				if got.db != nil {
					_ = got.db.Close()
				}
			}
		})
	}
}
