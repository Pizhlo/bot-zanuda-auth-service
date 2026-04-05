package audit

import (
	"auth-service/pkg/audit/internal"
	"time"
)

type fieldID internal.FieldID

//go:generate stringer -type=fieldID -trimprefix=field -output=field_id_string.go
const (
	_                fieldID = iota
	fieldCurrentTime         // время возникновения ошибки
	fieldServiceName         // имя сервиса
	fieldLevel               // уровень ошибки
	fieldMessage             // сообщение ошибки
	fieldErrorCode           // код ошибки
	fieldTraceID             // идентификатор трассировки. может быть null, так как пока не реализовано
	fieldRequestID           // идентификатор запроса. может быть null, так как пока не реализовано
	fieldUserID              // идентификатор пользователя (если есть)
	fieldStackTrace          // стек вызовов (если возможно)
	fieldContext             // контекст ошибки
	fieldVersion             // версия сервиса
	fieldKind                // вид ошибки
	fieldCause               // исходная ошибка
	fieldIPAddress           // ip адрес, с которого пришел запрос
	fieldOperation           // операция, которая была выполнена
	fieldStatus              // статус операции (started, completed, canceled, failed)
)

// currentTime создает новое поле текущего времени.
func currentTime() internal.Field {
	return internal.NewField(fieldCurrentTime, time.Now())
}

// ServiceName создает новое поле имени сервиса.
func ServiceName(serviceName string) internal.Field {
	return internal.NewField(fieldServiceName, serviceName)
}

// ErrorLevel - уровень ошибки (debug, info, error, etc).
//
//go:generate stringer -type=ErrorLevel -trimprefix=Err -output=error_level_string.go
type ErrorLevel int

// ErrorLevel - уровень ошибки.
const (
	// ErrLevelDebug - уровень ошибки: debug.
	ErrLevelDebug ErrorLevel = iota + 1
	// ErrLevelInfo - уровень ошибки: info.
	ErrLevelInfo
	// ErrLevelWarn - уровень ошибки: warn.
	ErrLevelWarn
	// ErrLevelError - уровень ошибки: error.
	ErrLevelError
	// ErrLevelFatal - уровень ошибки: fatal.
	ErrLevelFatal
	// ErrLevelPanic - уровень ошибки: panic.
	ErrLevelPanic
)

// Level создает новое поле уровня ошибки.
func Level(level ErrorLevel) internal.Field {
	return internal.NewField(fieldLevel, level)
}

// Message создает новое поле сообщения.
// С помощью него можно передать дополнительную информацию о событии.
func Message(message string) internal.Field {
	return internal.NewField(fieldMessage, message)
}

// ErrorCode - код ошибки.
type ErrorCode string

const (
	// ErrCodeServiceNotFound - код ошибки для случая, когда сервис не найден (неправильный client_id).
	ErrCodeServiceNotFound ErrorCode = "SERVICE_NOT_FOUND"
	// ErrCodeServiceUnavailable - код ошибки для случая, когда сервис недоступен (например, нет подключения к базе данных).
	ErrCodeServiceUnavailable ErrorCode = "SERVICE_UNAVAILABLE"
	// ErrCodeServiceInvalid - код ошибки для случая, когда сервис недействителен (например, неверный secret).
	ErrCodeServiceInvalid ErrorCode = "SERVICE_INVALID"
	// ErrCodeTokenInvalid - код ошибки для случая, когда токен невалиден (нет префикса Bearer или пустой токен, неверные claims).
	ErrCodeTokenInvalid ErrorCode = "AUTH_TOKEN_INVALID"
	// ErrCodeTokenExpired - код ошибки для случая, когда токен просрочен.
	ErrCodeTokenExpired ErrorCode = "AUTH_TOKEN_EXPIRED"
	// ErrCodePermDeniedSpace - код ошибки для случая, когда пользователь не имеет доступа к пространству.
	ErrCodePermDeniedSpace ErrorCode = "PERM_DENIED_SPACE"
	// ErrCodeInactiveClient - код ошибки для случая, когда машинный клиент не активен.
	ErrCodeInactiveClient ErrorCode = "CLIENT_INACTIVE"
	// ErrCodeInvalidSecret - код ошибки для случая, когда секрет машинного клиента неверный.
	ErrCodeInvalidSecret ErrorCode = "INVALID_SECRET"
	// ErrCodeVaultSecretNotFound - код ошибки для случая, когда секрет не найден в Vault.
	ErrCodeVaultSecretNotFound ErrorCode = "VAULT_SECRET_NOT_FOUND"
	// ErrCodeInvalidGrantType - код ошибки для случая, когда grant type неверный.
	ErrCodeInvalidGrantType ErrorCode = "INVALID_GRANT_TYPE"
	// ErrCodeInvalidScope - код ошибки для случая, когда scope неверный.
	ErrCodeInvalidScope ErrorCode = "INVALID_SCOPE"
	// ErrCodeTokenGenerationFailed - код ошибки для случая, когда токен не может быть сгенерирован.
	ErrCodeTokenGenerationFailed ErrorCode = "TOKEN_GENERATION_FAILED"
	// ErrCodeUserNotFound - код ошибки для случая, когда пользователь не найден.
	ErrCodeUserNotFound ErrorCode = "USER_NOT_FOUND"
	// ErrCodePanic - код ошибки для случая, когда произошла паника.
	ErrCodePanic ErrorCode = "PANIC"
)

// ErrorCodeField создает новое поле кода ошибки.
func ErrorCodeField(errorCode ErrorCode) internal.Field {
	return internal.NewField(fieldErrorCode, errorCode)
}

// TraceID создает новое поле идентификатора трассировки.
func TraceID(traceID string) internal.Field {
	return internal.NewField(fieldTraceID, traceID)
}

// RequestID создает новое поле идентификатора запроса.
func RequestID(requestID string) internal.Field {
	return internal.NewField(fieldRequestID, requestID)
}

// UserID создает новое поле пользователя.
func UserID(userID string) internal.Field {
	return internal.NewField(fieldUserID, userID)
}

// Operation создает новое поле операции.
func Operation(operation string) internal.Field {
	return internal.NewField(fieldOperation, operation)
}

// stackTrace создает новое поле стека вызовов.
func stackTrace(stackTrace string) internal.Field {
	return internal.NewField(fieldStackTrace, stackTrace)
}

// IPAddress создает новое поле IP-адреса.
func IPAddress(ipAddress string) internal.Field {
	return internal.NewField(fieldIPAddress, ipAddress)
}

// EventContext - контекст события.
// Может хранить в себе любые данные: url, по которому пришел запрос, метод, uuid пользователя, etc.
type EventContext map[string]any

// ContextField сохраняет новый контекст ошибки, добавляя его к уже существующему.
func ContextField(context EventContext, stash Stash) internal.Field {
	merged := make(EventContext)

	if existing, ok := stash.fields[fieldContext]; ok {
		if prev, ok := existing.Value.(EventContext); ok {
			for k, v := range prev {
				merged[k] = v
			}
		}
	}

	for k, v := range context {
		merged[k] = v
	}

	return internal.NewField(fieldContext, merged)
}

func version(version string) internal.Field {
	return internal.NewField(fieldVersion, version)
}

// Kind - тип ошибки.
type Kind int

//go:generate stringer -type=Kind -trimprefix=Kind -output=kind_string.go
const (
	// KindValidation - тип ошибки: validation.
	KindValidation Kind = iota + 1
	// KindDomain - тип ошибки: domain. Доменные ошибки - это ошибки, связанные с бизнес-логикой.
	KindDomain Kind = iota + 1
	// KindInfra - тип ошибки: infra. Инфраструктурные ошибки - это ошибки, связанные с инфраструктурой.
	KindInfra Kind = iota + 1
	// KindExternal - тип ошибки: external. Внешние ошибки - это ошибки, связанные с внешними системами.
	KindExternal Kind = iota + 1
	// KindInternal - тип ошибки: internal. Внутренние ошибки - это ошибки, связанные с внутренней логикой.
	KindInternal Kind = iota + 1
)

// KindField создает новое поле типа ошибки.
// Типы ошибок: validation, domain, infra, external, internal.
func KindField(kind Kind) internal.Field {
	return internal.NewField(fieldKind, kind)
}

func cause(cause error) internal.Field {
	return internal.NewField(fieldCause, cause)
}

// Status - статус операции (started, completed, canceled, failed).
type Status int

//go:generate stringer -type=Status -trimprefix=Status -output=status_string.go
const (
	// StatusStarted - операция только началась.
	StatusStarted Status = iota + 1
	// StatusCompleted - успешно выполненная операция.
	StatusCompleted
	// StatusCanceled - операция была отменена (например, при отмене контекста).
	StatusCanceled
	// StatusFailed - статус операции: failed. Для операций, которые завершились неудачно с т.з. бизнес-логики.
	StatusFailed
)

// StatusField создает новое поле статуса операции.
func StatusField(status Status) internal.Field {
	return internal.NewField(fieldStatus, status)
}
