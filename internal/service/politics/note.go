package politics

import (
	"auth-service/internal/model"
	"auth-service/internal/service"
	"auth-service/internal/service/internal"
	"auth-service/pkg/audit"
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

const (
	serviceName          = "politics"
	messageUserNotMember = "user is not member"
)

// FilterNotes фильтрует входящий список айди заметок, возвращая только доступные пользователю и существующие в пространстве.
//
// Пример: список [1, 2, 3, 4, 5, 6], в пространстве только: [1, 2, 3], из них доступны пользователю: [2, 3] -> вернется только [2, 3].
func (s *Service) FilterNotes(ctx context.Context, req model.FilterNotesRequest) (map[uuid.UUID]model.NoteAccessInfo, error) {
	operationFilterNotes := fmt.Sprintf("%s.%s", serviceName, "filter_notes")

	ctx = internal.WithOperation(ctx, operationFilterNotes)
	ctx = internal.WithUserID(ctx, req.UserID.String())
	ctx = withSpaceID(ctx, req.SpaceID)
	ctx = withNoteIDs(ctx, req.NoteIDs)
	ctx = internal.WithServiceName(ctx)

	event := s.auditor.Create(ctx)
	defer internal.WithPanicRecovery(ctx, event)()

	logrus.WithFields(logrus.Fields{
		"user_id":  req.UserID,
		"space_id": req.SpaceID,
		"note_ids": req.NoteIDs,
	}).Debug("filtering notes for space")

	spaceAccess, err := s.spaceAccessChecker.CheckSpaceAccess(ctx, req.UserID, req.SpaceID)
	if err != nil {
		event.WithError(audit.ErrCodeServiceUnavailable, audit.KindDomain, err)
		event.Append(audit.Level(audit.ErrLevelWarn))

		return nil, err
	}

	if !spaceAccess.IsMember {
		logrus.WithFields(logrus.Fields{
			"user_id":  req.UserID,
			"space_id": req.SpaceID,
			"note_ids": req.NoteIDs,
		}).Debugf("user is not member: access denied")

		event.WithError(audit.ErrCodePermDeniedSpace, audit.KindDomain, service.ErrUserNotMember)
		event.Append(audit.Message(messageUserNotMember))
		event.Append(audit.Level(audit.ErrLevelWarn))

		return map[uuid.UUID]model.NoteAccessInfo{}, service.ErrUserNotMember
	}

	existingIDs, err := s.storage.FilterNoteIDs(ctx, req.SpaceID, req.NoteIDs)
	if err != nil {
		event.WithError(audit.ErrCodeServiceUnavailable, audit.KindDomain, err)
		event.Append(audit.Level(audit.ErrLevelWarn))

		return nil, err
	}

	notes, err := s.notePermissionResolver.ResolveNotePermissions(ctx, spaceAccess, existingIDs)
	if err != nil {
		event.WithError(audit.ErrCodeServiceUnavailable, audit.KindDomain, err)
		event.Append(audit.Level(audit.ErrLevelWarn))

		return nil, err
	}

	return notes, nil
}

// FilterNotesFailedHook создает событие о невалидности токена.
func FilterNotesFailedHook(ctx context.Context, stash audit.Stash) audit.Stash {
	if serviceName, ok := ctx.Value(internal.ServiceNameKey{}).(string); ok {
		stash = stash.Append(audit.ServiceName(serviceName))
	}

	if message, ok := ctx.Value(internal.MessageKey{}).(string); ok {
		stash = stash.Append(audit.Message(message))
	}

	if level, ok := ctx.Value(internal.LevelKey{}).(audit.ErrorLevel); ok {
		stash = stash.Append(audit.Level(level))
	}

	if errorCode, ok := ctx.Value(internal.ErrorCodeKey{}).(audit.ErrorCode); ok {
		stash = stash.Append(audit.ErrorCodeField(errorCode))
	}

	if messageCtx, ok := ctx.Value(internal.MessageContextKey{}).(audit.EventContext); ok {
		stash = stash.Append(audit.ContextField(messageCtx, stash))
	}

	if userID, ok := ctx.Value(internal.UserIDKey{}).(string); ok {
		stash = stash.Append(audit.UserID(userID))
	}

	if operation, ok := ctx.Value(internal.OperationKey{}).(string); ok {
		stash = stash.Append(audit.Operation(operation))
	}

	if kind, ok := ctx.Value(internal.KindKey{}).(audit.Kind); ok {
		stash = stash.Append(audit.KindField(kind))
	}

	msgCtx := audit.EventContext{}

	if spaceID, ok := ctx.Value(spaceIDKey{}).(uuid.UUID); ok {
		msgCtx["space_id"] = spaceID.String()
	}

	if noteIDs, ok := ctx.Value(noteIDsKey{}).([]uuid.UUID); ok {
		msgCtx["note_ids"] = noteIDs
	}

	if len(msgCtx) > 0 {
		stash = stash.Append(audit.ContextField(msgCtx, stash))
	}

	return stash
}

type spaceIDKey struct{}

func withSpaceID(ctx context.Context, spaceID uuid.UUID) context.Context {
	return context.WithValue(ctx, spaceIDKey{}, spaceID)
}

type noteIDsKey struct{}

func withNoteIDs(ctx context.Context, noteIDs []uuid.UUID) context.Context {
	return context.WithValue(ctx, noteIDsKey{}, noteIDs)
}
