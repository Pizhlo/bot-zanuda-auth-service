package enforcer

import (
	"crypto/sha256"
	_ "embed" // Встраиваем неизменяемую политику в сборку.
	"fmt"
	"slices"
	"strings"

	"github.com/casbin/casbin/v2"
	casbinmodel "github.com/casbin/casbin/v2/model"
	stringadapter "github.com/casbin/casbin/v2/persist/string-adapter"
)

//go:embed fields/model.conf
var fieldModel string

//go:embed fields/policy.csv
var fieldPolicy string

type fieldEvaluator interface {
	Enforce(...interface{}) (bool, error)
}

// FieldPolicy содержит неизменяемую после загрузки политику Casbin для имён полей.
// Она вызывается после FGA allow и не предоставляет доступ к объекту сама.
// Одни и те же списки действуют для всех пользователей с правом update.
type FieldPolicy struct {
	evaluator fieldEvaluator
	version   string
}

// NewFieldPolicy загружает встроенные модель и CSV один раз, проверяя структуру правил.
// Ошибка должна блокировать запуск сервиса. Изменение политики требует новой сборки,
// runtime-редактирование и отдельный источник ролей не предусмотрены.
func NewFieldPolicy() (*FieldPolicy, error) { return loadFieldPolicy(fieldModel, fieldPolicy) }

func loadFieldPolicy(modelText, policyText string) (*FieldPolicy, error) {
	if strings.TrimSpace(modelText) == "" || strings.TrimSpace(policyText) == "" {
		return nil, fmt.Errorf("field model and policy are required")
	}

	parsed, err := casbinmodel.NewModelFromString(modelText)
	if err != nil {
		return nil, fmt.Errorf("parse field model: %w", err)
	}

	normalized, err := normalizeFieldRules(policyText)
	if err != nil {
		return nil, fmt.Errorf("normalize field rules: %w", err)
	}

	engine, err := casbin.NewEnforcer(parsed, stringadapter.NewAdapter(normalized))
	if err != nil {
		return nil, fmt.Errorf("load field policy: %w", err)
	}

	hash := sha256.Sum256([]byte(modelText + "\x00" + normalized))

	return &FieldPolicy{evaluator: engine, version: fmt.Sprintf("casbin:sha256:%x", hash)}, nil
}

// normalizeFieldRules исключает пустые строки и комментарии, проверяет каждое правило
// и сортирует их для стабильной версии политики независимо от порядка строк.
func normalizeFieldRules(policyText string) (string, error) {
	var rules []string

	for line := range strings.SplitSeq(policyText, "\n") {
		rule := strings.TrimSpace(line)
		if rule == "" || strings.HasPrefix(rule, "#") {
			continue
		}

		parts := strings.Split(rule, ",")
		if len(parts) != 4 || strings.TrimSpace(parts[0]) != "p" {
			return "", fmt.Errorf("field policy requires resource, action and field")
		}

		for _, part := range parts[1:] {
			if strings.TrimSpace(part) == "" {
				return "", fmt.Errorf("field policy contains an empty parameter")
			}
		}

		rules = append(rules, rule)
	}

	if len(rules) == 0 {
		return "", fmt.Errorf("field policy contains no rules")
	}

	slices.Sort(rules)

	return strings.Join(rules, "\n"), nil
}

// Version возвращает версию фактически загруженных модели и правил.
func (p *FieldPolicy) Version() string { return p.version }

// CheckFields проверяет всю маску, возвращая отсортированные запрещённые имена без дублей.
// Исходная маска не изменяется; пустая маска и ошибка движка возвращаются как ошибка.
// Вызывающий код разрешает update только при пустом списке запрещённых полей.
func (p *FieldPolicy) CheckFields(resource, action string, fields []string) ([]string, error) {
	if len(fields) == 0 {
		return nil, fmt.Errorf("field mask must not be empty")
	}

	unique := slices.Clone(fields)
	slices.Sort(unique)
	unique = slices.Compact(unique)

	forbidden := make([]string, 0, len(unique))
	for _, field := range unique {
		allowed, err := p.evaluator.Enforce(resource, action, field)
		if err != nil {
			return nil, fmt.Errorf("evaluate field policy: %w", err)
		}

		if !allowed {
			forbidden = append(forbidden, field)
		}
	}

	return forbidden, nil
}
