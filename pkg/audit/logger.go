package audit

import (
	"log/slog"
	"os"
)

// LevelAudit - уровень аудита.
// Значение выбрано нарочно между error и warn, чтобы было понятно, что это аудит.
//
//nolint:gochecknoglobals // константное значение.
var LevelAudit slog.Level = 6

// levelNames - мапа для сопоставления уровней slog.Level с их строковыми представлениями.
//
//nolint:gochecknoglobals // константное значение.
var levelNames = map[slog.Level]string{
	slog.LevelDebug: "DEBUG",
	slog.LevelInfo:  "INFO",
	slog.LevelWarn:  "WARN",
	slog.LevelError: "ERROR",
	LevelAudit:      "AUDIT", // Наш кастомный уровень
}

// InitLogger инициализирует логгер для аудита.
func InitLogger() *slog.Logger {
	opts := &slog.HandlerOptions{
		Level: LevelAudit,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.LevelKey {
				a.Key = "level" // Можно переименовать ключ, если нужно (например, "sev")

				level, ok := a.Value.Any().(slog.Level)
				if !ok {
					return a
				}

				name, ok := levelNames[level]
				if !ok {
					name = level.String() // Fallback для неизвестных
				}

				a.Value = slog.StringValue(name)
			}
			// Убираем время для чистоты примера (опционально)
			if a.Key == slog.TimeKey {
				return slog.Attr{}
			}

			return a
		},
	}

	handler := slog.NewTextHandler(os.Stdout, opts) // Или NewTextHandler для текста
	logger := slog.New(handler)

	return logger
}
