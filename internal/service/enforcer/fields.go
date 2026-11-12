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

// FieldPolicy проверяет поля, не предоставляя прав на объект и не храня роли.
type FieldPolicy struct {
	evaluator fieldEvaluator
	version   string
}

// NewFieldPolicy загружает встроенные правила; ошибка блокирует запуск сервиса.
func NewFieldPolicy() (*FieldPolicy, error) { return loadFieldPolicy(fieldModel, fieldPolicy) }

func loadFieldPolicy(modelText, policyText string) (*FieldPolicy, error) {
	if strings.TrimSpace(modelText) == "" || strings.TrimSpace(policyText) == "" {
		return nil, fmt.Errorf("field model and policy are required")
	}

	parsed, err := casbinmodel.NewModelFromString(modelText)
	if err != nil {
		return nil, fmt.Errorf("parse field model: %w", err)
	}

	rules := strings.Split(strings.TrimSpace(policyText), "\n")
	for i := range rules {
		rules[i] = strings.TrimSpace(rules[i])

		parts := strings.Split(rules[i], ",")
		if len(parts) != 4 || strings.TrimSpace(parts[0]) != "p" {
			return nil, fmt.Errorf("field policy requires resource, action and field")
		}

		for _, part := range parts[1:] {
			if strings.TrimSpace(part) == "" {
				return nil, fmt.Errorf("field policy contains an empty parameter")
			}
		}
	}

	slices.Sort(rules)
	normalized := strings.Join(rules, "\n")

	engine, err := casbin.NewEnforcer(parsed, stringadapter.NewAdapter(normalized))
	if err != nil {
		return nil, fmt.Errorf("load field policy: %w", err)
	}

	hash := sha256.Sum256([]byte(modelText + "\x00" + normalized))

	return &FieldPolicy{evaluator: engine, version: fmt.Sprintf("casbin:sha256:%x", hash)}, nil
}

// Version возвращает версию фактически загруженных модели и правил.
func (p *FieldPolicy) Version() string { return p.version }

// CheckFields возвращает отсортированные запрещённые поля; ошибка не является deny.
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
