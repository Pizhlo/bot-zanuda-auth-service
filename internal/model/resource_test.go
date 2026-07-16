package model

import (
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := FormatEventType(tt.eventTypePrefix, tt.eventTypePostifx)
			require.Equal(t, tt.want, got)
		})
	}
}
