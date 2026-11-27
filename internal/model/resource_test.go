//nolint:funlen // тестовые функции могут быть любой длины
package model

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestResource_ParentRelationName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		resource Resource
		want     string
		wantErr  require.ErrorAssertionFunc
	}{
		{
			name:     "note",
			resource: Resource{ID: uuid.New(), Type: ResourceTypeNote},
			want:     "space",
			wantErr:  require.NoError,
		},
		{
			name:     "reminder",
			resource: Resource{ID: uuid.New(), Type: ResourceTypeReminder},
			want:     "space",
			wantErr:  require.NoError,
		},
		{
			name:     "space",
			resource: Resource{ID: uuid.New(), Type: ResourceTypeSpace},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "resource type space does not support parent relation")
			},
		},
		{
			name:     "user",
			resource: Resource{ID: uuid.New(), Type: ResourceTypeUser},
			wantErr: func(t require.TestingT, err error, i ...interface{}) {
				require.Error(t, err)
				require.ErrorContains(t, err, "resource type user does not support parent relation")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := tt.resource.ParentRelationName()
			tt.wantErr(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestResource_IsEmpty(t *testing.T) {
	t.Parallel()

	r := Resource{ID: uuid.New(), Type: ResourceTypeNote}
	require.False(t, r.IsEmpty())

	r = Resource{ID: uuid.Nil, Type: ""}
	require.True(t, r.IsEmpty())

	r = Resource{ID: uuid.Nil, Type: ResourceTypeNote}
	require.True(t, r.IsEmpty())

	r = Resource{ID: uuid.New(), Type: ""}
	require.True(t, r.IsEmpty())
}

func TestChangeType_IsOneOf(t *testing.T) {
	t.Parallel()

	c := ChangeTypeResourceAdded
	require.True(t, c.IsOneOf(ChangeTypeResourceAdded, ChangeTypeResourceRemoved, ChangeTypeResourceMoved))
	require.False(t, c.IsOneOf(ChangeTypeMembershipChanged, ChangeTypeMembershipAdded, ChangeTypeMembershipRemoved))
}

func TestFormatEventType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		eventTypePrefix  string
		eventTypePostifx string
		want             string
	}{
		{
			name:             "note_created",
			eventTypePrefix:  EventTypePrefixNote,
			eventTypePostifx: EventTypeOperationCreatedPostfix,
			want:             "NOTE_CREATED",
		},
		{
			name:             "note_updated",
			eventTypePrefix:  EventTypePrefixNote,
			eventTypePostifx: EventTypeOperationUpdatedPostfix,
			want:             "NOTE_UPDATED",
		},
		{
			name:             "note_deleted",
			eventTypePrefix:  EventTypePrefixNote,
			eventTypePostifx: EventTypeOperationDeletedPostfix,
			want:             "NOTE_DELETED",
		},
		{
			name:             "reminder_created",
			eventTypePrefix:  EventTypePrefixReminder,
			eventTypePostifx: EventTypeOperationCreatedPostfix,
			want:             "REMINDER_CREATED",
		},
		{
			name:             "reminder_updated",
			eventTypePrefix:  EventTypePrefixReminder,
			eventTypePostifx: EventTypeOperationUpdatedPostfix,
			want:             "REMINDER_UPDATED",
		},
		{
			name:             "reminder_deleted",
			eventTypePrefix:  EventTypePrefixReminder,
			eventTypePostifx: EventTypeOperationDeletedPostfix,
			want:             "REMINDER_DELETED",
		},
		{
			name:             "space_created",
			eventTypePrefix:  EventTypePrefixSpace,
			eventTypePostifx: EventTypeOperationCreatedPostfix,
			want:             "SPACE_CREATED",
		},
		{
			name:             "space_updated",
			eventTypePrefix:  EventTypePrefixSpace,
			eventTypePostifx: EventTypeOperationUpdatedPostfix,
			want:             "SPACE_UPDATED",
		},
		{
			name:             "space_deleted",
			eventTypePrefix:  EventTypePrefixSpace,
			eventTypePostifx: EventTypeOperationDeletedPostfix,
			want:             "SPACE_DELETED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := FormatEventType(tt.eventTypePrefix, tt.eventTypePostifx)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestEqual(t *testing.T) {
	t.Parallel()

	id := uuid.New()

	owner := Resource{Type: ResourceTypeUser, ID: id}
	for _, tt := range []struct {
		name  string
		other Resource
		equal bool
	}{
		{"same identity", Resource{Type: ResourceTypeUser, ID: id}, true},
		{"attributes are not identity", Resource{Type: ResourceTypeUser, ID: id, Attributes: map[string]any{"name": "changed"}}, true},
		{"different type", Resource{Type: ResourceTypeSpace, ID: id}, false},
		{"different uuid", Resource{Type: ResourceTypeUser, ID: uuid.New()}, false},
		{"empty", Resource{}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.equal, owner.Equal(tt.other))

			require.Equal(t, tt.equal, tt.other.Equal(owner))
		})
	}
}

func TestParseResourceIdentity(t *testing.T) {
	t.Parallel()

	id := uuid.MustParse("f3172ef4-9df7-4d96-b386-3cc00f6572dc")

	for _, tc := range []struct {
		name, raw string
		want      Resource
		wantErr   require.ErrorAssertionFunc
	}{
		{"valid identity", "user:" + id.String(), Resource{Type: ResourceTypeUser, ID: id}, require.NoError},
		{"missing identity", "", Resource{}, require.Error},
		{"missing kind", ":" + id.String(), Resource{}, require.Error},
		{"wildcard", "user:*", Resource{}, require.Error},
		{"invalid UUID", "user:bad", Resource{}, require.Error},
		{"zero UUID", "user:" + uuid.Nil.String(), Resource{}, require.Error},
		{"userset", "space:" + id.String() + "#viewer", Resource{}, require.Error},
		{"UUID without dashes", "user:f3172ef49df74d96b3863cc00f6572dc", Resource{}, require.Error},
		{"uppercase UUID", "user:F3172EF4-9DF7-4D96-B386-3CC00F6572DC", Resource{}, require.Error},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseResourceIdentity(tc.raw)
			tc.wantErr(t, err)

			require.Equal(t, tc.want, got)
		})
	}
}

func TestUpdateResourceResponse(t *testing.T) {
	t.Parallel()

	data, err := json.Marshal(UpdateResourceResponse{})
	require.NoError(t, err)

	require.Contains(t, string(data), `"auth_model_id"`)
	require.NotContains(t, string(data), `"authorization_model_id"`)
}
