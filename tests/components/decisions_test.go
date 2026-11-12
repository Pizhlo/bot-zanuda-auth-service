//go:build integration

package components_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"auth-service/internal/config"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/require"
)

// TestDecisionWireChecks проверяет, какое право сервис запрашивает у OpenFGA при остановке
// напоминания и создании первого личного пространства. Для остановки учитывается выбранная
// согласованность, а отметку владельца всегда читают с повышенной согласованностью.
// В каждом случае ожидаются один запрос к закреплённой модели и соответствующая причина разрешения.
func TestDecisionWireChecks(t *testing.T) {
	f := newFixture(t)
	spy := observe(t)
	f.launch(t, "/app/main", spy.url).ready(t)
	token := login(t)
	reminder := uuid.NewString()
	f.store.write(t, []key{{"space:" + f.space, "space", "reminder:" + reminder}})
	for _, tc := range []struct {
		name, action, kind, id, consistency, relation, wantMode string
		attrs                                                   object
	}{
		{"default stop", "stop", "reminder", reminder, "", "can_stop", "HIGHER_CONSISTENCY", object{}},
		{"consistent stop", "stop", "reminder", reminder, "higher_consistency", "can_stop", "HIGHER_CONSISTENCY", object{}},
		{"latency stop", "stop", "reminder", reminder, "minimize_latency", "can_stop", "MINIMIZE_LATENCY", object{}},
		{"first personal", "create", "space", uuid.NewString(), "minimize_latency", "has_personal_space", "HIGHER_CONSISTENCY", object{"space_type": "personal"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spy.reset()
			body := f.body(tc.action, tc.kind, tc.id, tc.attrs, object{})
			if tc.consistency != "" {
				body["options"].(object)["consistency"] = tc.consistency
			}
			result := f.check(t, token, body, 200)
			require.Equal(t, "allow", result["result"])
			resource := tc.kind + ":" + tc.id
			reason := "allowed_via_fga_relation"
			if tc.action == "create" {
				resource = "user:" + f.owner
				reason = "first_personal_space"
			}
			reasons := result["reasons"].([]any)
			require.Len(t, reasons, 1)
			require.Equal(t, reason, reasons[0].(object)["code"])
			require.NotEmpty(t, reasons[0].(object)["message"])
			calls := spy.take("/check")
			require.Len(t, calls, 1)
			require.Equal(t, object{"user": "user:" + f.owner, "relation": tc.relation, "object": resource}, calls[0]["tuple_key"])
			require.Equal(t, tc.wantMode, calls[0]["consistency"])
			require.Equal(t, f.store.model, calls[0]["authorization_model_id"])
			require.Equal(t, result["meta"].(object)["authorization_model_id"], calls[0]["authorization_model_id"])
		})
	}
}

// TestEmbeddedPolicyReadinessAndVersion проверяет, что сервис с пустой встроенной политикой
// завершается с ошибкой до готовности принимать запросы. Для рабочей политики версия в ответе
// авторизации воспроизводится после перезапуска и меняется вместе с содержимым политики.
func TestEmbeddedPolicyReadinessAndVersion(t *testing.T) {
	f := newFixture(t)
	p := f.launch(t, "/components/empty", fgaURL)
	timeout := time.After(20 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
waiting:
	for {
		select {
		case <-p.done:
			require.Error(t, p.err)
			log, err := os.ReadFile(p.logfile)
			require.NoError(t, err)
			require.True(t, strings.Contains(string(log), "field model and policy are required"), "expected embedded policy startup error")
			break waiting
		case <-timeout:
			t.Fatal("empty embedded policy did not prevent startup")
		case <-ticker.C:
			require.False(t, healthy(), "empty policy reached readiness")
		}
	}
	versions := make([]string, 0, 3)
	for _, binary := range []string{"/app/main", "/app/main", "/components/changed"} {
		service := f.launch(t, binary, fgaURL)
		service.ready(t)
		result := f.check(t, login(t), f.body("read", "space", f.space, object{}, object{}), 200)
		require.Equal(t, "allow", result["result"])
		version := result["meta"].(object)["policy_version"].(string)
		require.Regexp(t, "^casbin:sha256:[0-9a-f]{64}$", version)
		versions = append(versions, version)
		service.stop(t)
	}
	require.Equal(t, versions[0], versions[1])
	require.NotEqual(t, versions[0], versions[2])
}

// TestPolicyUnavailable проверяет сбой вычисления политики при изменении имени пространства.
// Клиент получает HTTP 503 с кодом policy_unavailable, а RabbitMQ — событие о сбое проверки
// полей с тем же идентификатором запроса. Повторная доставка события в контрольном окне
// и запись завершённого решения в PostgreSQL исключены.
func TestPolicyUnavailable(t *testing.T) {
	f := newFixture(t)
	f.launch(t, "/components/unavailable", fgaURL).ready(t)
	body := f.body("update", "space", f.space, object{}, object{"field_mask": []string{"name"}})
	result := f.check(t, login(t), body, 503)
	require.Equal(t, object{"request_id": body["request_id"], "error_code": "policy_unavailable", "message": "Field policy is unavailable"}, result)
	cfg, err := config.LoadConfig("/components.yaml")
	require.NoError(t, err)
	conn, err := amqp.Dial(cfg.RabbitMQ.URL)
	require.NoError(t, err)
	defer conn.Close()
	ch, err := conn.Channel()
	require.NoError(t, err)
	defer ch.Close()
	deliveries, err := ch.Consume(cfg.Audit.Topic, "", true, false, false, false, nil)
	require.NoError(t, err)

	requestID := body["request_id"].(string)
	event := nextAuditEvent(t, deliveries, requestID, 10*time.Second)
	require.NotNil(t, event, "expected policy_unavailable audit event for %s", requestID)
	require.Equal(t, "check_decision", event["operation"])
	require.Equal(t, "policy_unavailable", event["error_code"])
	require.Equal(t, "Field policy is unavailable", event["message"])
	require.Equal(t, "failed", event["status"])
	detail := event["context"].(object)
	require.Equal(t, "check_fields", detail["stage"])
	require.NotContains(t, detail, "decision_id")
	require.NotContains(t, detail, "decision")
	duplicate := nextAuditEvent(t, deliveries, requestID, 300*time.Millisecond)
	require.Nil(t, duplicate, "duplicate audit event for %s", requestID)

	var count int
	require.NoError(t, f.db.QueryRowContext(t.Context(), "SELECT count(*) FROM auth.decisions WHERE request_id=$1", body["request_id"]).Scan(&count))
	require.Zero(t, count)
}

// nextAuditEvent ждёт событие данного запроса до общего дедлайна. События других
// запросов не продлевают ожидание, а закрытие канала означает ошибку наблюдения.
func nextAuditEvent(t *testing.T, deliveries <-chan amqp.Delivery, requestID string, timeout time.Duration) object {
	t.Helper()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			return nil
		case delivery, open := <-deliveries:
			require.True(t, open, "RabbitMQ closed the audit consumer while waiting for %s", requestID)
			var event object
			require.NoError(t, json.Unmarshal(delivery.Body, &event))
			if event["request_id"] == requestID {
				return event
			}
		}
	}
}
