package audit

import (
	"auth-service/pkg/audit/internal"
	"fmt"
	"time"
)

type fieldID internal.FieldID

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

// MarshalText реализует [encoding.TextMarshaler] для fieldID.
func (f fieldID) MarshalText() (text []byte, err error) {
	return []byte(f.String()), nil // используем stringer!
}

// String реализует [fmt.Stringer] для fieldID.
// Возвращает строковое представление fieldID в формате snake_case.
//
//nolint:goconst,cyclop // нет смысла выводить в константы строковые значения; много кейсов из-за множества значений.
func (f fieldID) String() string {
	switch f {
	case fieldCurrentTime:
		return "current_time"
	case fieldServiceName:
		return "service_name"
	case fieldLevel:
		return "level"
	case fieldMessage:
		return "message"
	case fieldErrorCode:
		return "error_code"
	case fieldTraceID:
		return "trace_id"
	case fieldRequestID:
		return "request_id"
	case fieldUserID:
		return "user_id"
	case fieldStackTrace:
		return "stack_trace"
	case fieldContext:
		return "context"
	case fieldVersion:
		return "version"
	case fieldKind:
		return "kind"
	case fieldCause:
		return "cause"
	case fieldIPAddress:
		return "ip_address"
	case fieldOperation:
		return "operation"
	case fieldStatus:
		return "status"
	default:
		return "unknown"
	}
}

// UnmarshalText реализует [encoding.TextUnmarshaler] для fieldID.
//
//nolint:cyclop // много кейсов из-за множества значений
func (f *fieldID) UnmarshalText(text []byte) error {
	s := string(text)
	// Обратный маппинг из string → fieldID
	switch s {
	case "current_time":
		*f = fieldCurrentTime
	case "service_name":
		*f = fieldServiceName
	case "level":
		*f = fieldLevel
	case "message":
		*f = fieldMessage
	case "error_code":
		*f = fieldErrorCode
	case "trace_id":
		*f = fieldTraceID
	case "request_id":
		*f = fieldRequestID
	case "user_id":
		*f = fieldUserID
	case "stack_trace":
		*f = fieldStackTrace
	case "context":
		*f = fieldContext
	case "version":
		*f = fieldVersion
	case "kind":
		*f = fieldKind
	case "cause":
		*f = fieldCause
	case "ip_address":
		*f = fieldIPAddress
	case "operation":
		*f = fieldOperation
	case "status":
		*f = fieldStatus
	default:
		return fmt.Errorf("unknown fieldID: %s", s)
	}

	return nil
}

// currentTime создает новое поле текущего времени.
func currentTime() internal.Field {
	return internal.NewField(fieldCurrentTime, time.Now())
}

// ServiceName создает новое поле имени сервиса.
func ServiceName(serviceName string) internal.Field {
	return internal.NewField(fieldServiceName, serviceName)
}

// ErrorLevel - уровень ошибки (debug, info, error, etc).
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

// MarshalText реализует [encoding.TextMarshaler] для ErrorLevel.
func (e ErrorLevel) MarshalText() ([]byte, error) {
	return []byte(e.String()), nil
}

// String реализует [fmt.Stringer] для ErrorLevel.
func (e ErrorLevel) String() string {
	switch e {
	case ErrLevelDebug:
		return "debug"
	case ErrLevelInfo:
		return "info"
	case ErrLevelWarn:
		return "warn"
	case ErrLevelError:
		return "error"
	case ErrLevelFatal:
		return "fatal"
	case ErrLevelPanic:
		return "panic"
	default:
		return "unknown"
	}
}

// UnmarshalText реализует [encoding.TextUnmarshaler] для ErrorLevel.
func (e *ErrorLevel) UnmarshalText(text []byte) error {
	s := string(text)
	switch s {
	case "debug":
		*e = ErrLevelDebug
	case "info":
		*e = ErrLevelInfo
	case "warn":
		*e = ErrLevelWarn
	case "error":
		*e = ErrLevelError
	case "fatal":
		*e = ErrLevelFatal
	case "panic":
		*e = ErrLevelPanic
	default:
		return fmt.Errorf("unknown ErrorLevel: %s", string(text))
	}

	return nil
}

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
	// ErrCodeEmptyLoginRequest - код ошибки для случая, когда запрос на авторизацию пуст.
	ErrCodeEmptyLoginRequest ErrorCode = "EMPTY_LOGIN_REQUEST"
	// ErrCodeInvalidScope - код ошибки для случая, когда scope неверный.
	ErrCodeInvalidScope ErrorCode = "INVALID_SCOPE"
	// ErrCodeTokenGenerationFailed - код ошибки для случая, когда токен не может быть сгенерирован.
	ErrCodeTokenGenerationFailed ErrorCode = "TOKEN_GENERATION_FAILED"
	// ErrCodeUserNotFound - код ошибки для случая, когда пользователь не найден.
	ErrCodeUserNotFound ErrorCode = "USER_NOT_FOUND"
	// ErrCodeOwnerRequired - код ошибки для случая, когда owner required.
	ErrCodeOwnerRequired ErrorCode = "OWNER_REQUIRED"
	// ErrCodeParentRequired - код ошибки для случая, когда parent required.
	ErrCodeParentRequired ErrorCode = "PARENT_REQUIRED"
	// ErrCodeChangeTypeInvalid - код ошибки для случая, когда change type invalid.
	ErrCodeChangeTypeInvalid ErrorCode = "CHANGE_TYPE_INVALID"
	// ErrCodeOperationInvalid - код ошибки для случая, когда operation invalid.
	ErrCodeOperationInvalid ErrorCode = "OPERATION_INVALID"
	// ErrCodeEventTypeInvalid - код ошибки для случая, когда event type invalid.
	ErrCodeEventTypeInvalid ErrorCode = "EVENT_TYPE_INVALID"
	// ErrCodeResourceEmpty - код ошибки для случая, когда resource empty.
	ErrCodeResourceEmpty ErrorCode = "RESOURCE_EMPTY"
	// ErrCodeOwnerTypeInvalid - код ошибки для случая, когда owner type invalid.
	ErrCodeOwnerTypeInvalid ErrorCode = "OWNER_TYPE_INVALID"
	// ErrCodeParentTypeInvalid - код ошибки для случая, когда parent type invalid.
	ErrCodeParentTypeInvalid ErrorCode = "PARENT_TYPE_INVALID"
	// ErrCodePanic - код ошибки для случая, когда произошла паника.
	ErrCodePanic ErrorCode = "PANIC"
	// ErrCodeWriteFailedDueToInvalidInput - код ошибки для случая, когда write failed due to invalid input.
	ErrCodeWriteFailedDueToInvalidInput ErrorCode = "WRITE_FAILED_DUE_TO_INVALID_INPUT"
	// ErrCodeResourceAlreadyExistsOrNotFound - код ошибки для случая, когда ресурс уже существует или не найден.
	ErrCodeResourceAlreadyExistsOrNotFound ErrorCode = "RESOURCE_ALREADY_EXISTS_OR_NOT_FOUND"
	ErrNoTuplesToWriteOrDelete             ErrorCode = "NO_TUPLES_TO_WRITE_OR_DELETE"
	ErrInternalServerError                 ErrorCode = "INTERNAL_SERVER_ERROR"
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

// MarshalText реализует [encoding.TextMarshaler] для Kind.
func (k Kind) MarshalText() ([]byte, error) {
	return []byte(k.String()), nil
}

// String реализует [fmt.Stringer] для Kind.
func (k Kind) String() string {
	switch k {
	case KindValidation:
		return "validation"
	case KindDomain:
		return "domain"
	case KindInfra:
		return "infra"
	case KindExternal:
		return "external"
	case KindInternal:
		return "internal"
	default:
		return "unknown"
	}
}

// UnmarshalText реализует [encoding.TextUnmarshaler] для Kind.
func (k *Kind) UnmarshalText(text []byte) error {
	s := string(text)
	switch s {
	case "validation":
		*k = KindValidation
	case "domain":
		*k = KindDomain
	case "infra":
		*k = KindInfra
	case "external":
		*k = KindExternal
	case "internal":
		*k = KindInternal
	default:
		return fmt.Errorf("unknown Kind: %s", s)
	}

	return nil
}

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

// MarshalText реализует [encoding.TextMarshaler] для Status.
func (s Status) MarshalText() ([]byte, error) {
	return []byte(s.String()), nil
}

// String реализует [fmt.Stringer] для Status.
func (s Status) String() string {
	switch s {
	case StatusStarted:
		return "started"
	case StatusCompleted:
		return "completed"
	case StatusCanceled:
		return "canceled"
	case StatusFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// UnmarshalText реализует [encoding.TextUnmarshaler] для Status.
func (s *Status) UnmarshalText(text []byte) error {
	sStr := string(text)
	switch sStr {
	case "started":
		*s = StatusStarted
	case "completed":
		*s = StatusCompleted
	case "canceled":
		*s = StatusCanceled
	case "failed":
		*s = StatusFailed
	default:
		return fmt.Errorf("unknown Status: %s", sStr)
	}

	return nil
}
