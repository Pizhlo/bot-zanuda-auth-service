#!/bin/sh

#
# Скрипт для инициализации очередей в RabbitMQ.
# Подготавливает RabbitMQ - создает DLX, DLQ, policy и очередь errors.auth-service.
#
set -eu

RABBITMQ_API="${RABBITMQ_API:-http://localhost:15672}"
RABBITMQ_USER="${RABBITMQ_USER:-guest}"
RABBITMQ_PASSWORD="${RABBITMQ_PASSWORD:-guest}"
AUTH="${RABBITMQ_USER}:${RABBITMQ_PASSWORD}"

echo '🐰 Waiting for RabbitMQ management API...'
attempts=0
until curl -fsS "${RABBITMQ_API}/api/aliveness-test/%2F" -u "${AUTH}" > /dev/null; do
  attempts=$((attempts + 1))
  if [ "${attempts}" -ge 20 ]; then
    echo '❌ RabbitMQ management API did not become ready'
    exit 1
  fi
  sleep 3
done

echo '🔨 Creating infrastructure...'

curl -sS --fail-with-body -u "${AUTH}" -X PUT "${RABBITMQ_API}/api/exchanges/%2F/dlx.errors" \
  -H 'Content-Type: application/json' \
  -d '{"type":"fanout","durable":true}' > /dev/null
echo '  ✅ DLX dlx.errors created'

curl -sS --fail-with-body -u "${AUTH}" -X PUT "${RABBITMQ_API}/api/queues/%2F/errors-dlq" \
  -H 'Content-Type: application/json' \
  -d '{"durable":true}' > /dev/null
echo '  ✅ DLQ errors-dlq created'

curl -sS --fail-with-body -u "${AUTH}" -X PUT "${RABBITMQ_API}/api/queues/%2F/errors.auth-service" \
  -H 'Content-Type: application/json' \
  -d '{"durable":true,"arguments":{"x-queue-type":"quorum"}}' > /dev/null
echo '  ✅ Queue errors.auth-service created'

curl -sS --fail-with-body -u "${AUTH}" -X POST \
  "${RABBITMQ_API}/api/bindings/%2F/e/dlx.errors/q/errors-dlq" > /dev/null
echo '  ✅ Binding dlx.errors → errors-dlq created'

curl -sS --fail-with-body -u "${AUTH}" -X PUT "${RABBITMQ_API}/api/policies/%2F/DLX-errors" \
  -H 'Content-Type: application/json' \
  -d '{
    "pattern": "^errors\\.",
    "definition": {
      "dead-letter-exchange": "dlx.errors",
      "delivery-limit": 3,
      "message-ttl": 604800000
    },
    "priority": 0,
    "apply-to": "quorum_queues"
  }' > /dev/null
echo '  ✅ Policy DLX-errors (^errors\.) created'

echo '✅ Setup complete!'

# Очистить очередь перед тестами (остатки от прошлых прогонов).
curl -sS --fail-with-body -u "${AUTH}" -X DELETE \
  "${RABBITMQ_API}/api/queues/%2F/errors.auth-service/contents" > /dev/null || true
