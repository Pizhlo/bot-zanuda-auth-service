# Ручная проверка BZ-167 через Postman

Итоговый endpoint: `POST {{base_url}}/api/v0/decisions/check`. Маршрут зарегистрирован через существующую группу `apiv0`; `/v0/decisions/check` не является алиасом. Проверка возвращает решение и ничего не меняет в заметке, напоминании или отношениях.

# TMP DIR: /tmp/bz167-migration.T0JWjM

## Подготовка

1. Подними привычное локальное окружение и сервис. Применённая строгая FGA-модель должна соответствовать `fga/auth-model.fga` текущей сборки. После изменения модели опубликуй её штатным инструментом и укажи полученный `openfga.model_id`; старый model ID не пройдёт `EnsureDecisionModel`. Для существующих пространств используй [порядок миграции](../space-types-migration.md).
2. Нужен активный сервисный клиент, например `bot`, с разрешённым scope `bot` и соответствующим секретом в Vault.
3. Нужен уже зарегистрированный Telegram-пользователь: его Telegram ID в заголовке должен соответствовать внутреннему UUID `subject.id` в PostgreSQL. Login выдаёт сервисный токен, но не создаёт этого пользователя. Для проверок ролей подготовь отдельных owner/editor/viewer через штатные тестовые данные внешнего Python-репозитория.
4. Для положительных проверок в FGA должны существовать owner пространства, его маркер `shared` или `personal`, parent-связь записи с пространством и owner записи. Доступ к настоящим текстам заметок сервису не нужен.
5. В Postman выбери Import и импортируй [BZ-167.postman_collection.json](BZ-167.postman_collection.json). Заполни переменные коллекции:

| Переменная | Значение |
|---|---|
| `base_url` | Адрес запущенного auth-service, например `http://localhost:8080`; порт соответствует `server.port` |
| `client_id`, `client_secret` | Данные сервисного клиента |
| `telegram_user_id`, `subject_id` | Согласованная пара Telegram ID и внутреннего UUID проверяемого пользователя |
| `shared_space_id`, `personal_space_id` | UUID существующих общего и личного пространств |
| `note_id`, `reminder_id` | UUID записей внутри общего пространства |
| `space_owner_id`, `target_user_id` | UUID владельца общего пространства и другого участника |
| `other_subject_id`, `other_telegram_user_id` | Другие корректные идентификаторы для отрицательных проверок |

ID ресурсов — полноценные UUID, не `n_1001` или `s_10`. Секреты и токены заполняй локально в Postman; шаблон коллекции их не содержит.

В запросе **07** `target_user_id` подставляется в `resource.id` — UUID пользователя, чью роль проверяем. Если переменная пустая или не подставилась, ответ будет `400` с `message: "resource.id must be a nonzero UUID"`. После заполнения UUID этот запрос по-прежнему должен вернуть `400`, но уже с `message: "action.attributes.role must be viewer, editor or admin for member add/edit"`: он специально проверяет запрещённое назначение `owner`.

Конфигурационные бюджеты можно задать так; при отсутствии полей или нулевых значениях используются эти же defaults:

```yaml
openfga:
  request_timeout: 5s
audit:
  delivery_timeout: 1s
```

Это дополнения к существующим разделам, а не полный конфигурационный файл. Отрицательные значения отклоняются при загрузке. Более ранний deadline вызывающего кода имеет приоритет.

## Получить токен

Отправь запрос Login из коллекции:

```http
POST {{base_url}}/api/v0/auth/login
Content-Type: application/json
```

```json
{
  "grant_type": "client_credentials",
  "client_id": "{{client_id}}",
  "client_secret": "{{client_secret}}",
  "scope": "bot"
}
```

Из ответа HTTP 200 скопируй `access_token` в переменную `service_token`. Проверки наследуют Bearer Auth коллекции; префикс `Bearer` вручную в эту переменную добавлять не надо.

## Создание пространства пользователем

Запросы 13–15 в коллекции проверяют `space:create` с новым UUID ресурса. Используй тот же сервисный токен и пару `telegram_user_id` / `subject_id`. В запросе 13 `resource.attributes.space_type` равен `shared` и ожидается HTTP 200 с `allow: true`. В запросе 14 тип `personal` даёт HTTP 200. Для владельца уже синхронизированного личного пространства ожидай `allow: false` и `personal_space_already_exists`. Для пользователя без личного пространства ожидай `allow: true` и `first_personal_space`. Повтор одного check до синхронизации даст тот же результат. В запросе 15 тип пропущен и ожидается HTTP 400 с именем поля. Синхронизация личного пространства, которое бизнес-система создала при регистрации, выполняется через `resources/update`.

Для каждого запроса в папке «Проверки» Postman сам подставляет один новый UUID в заголовок `X-Request-ID` и поле `request_id` тела. Если токен просрочен, ищи отказ по UUID из заголовка ответа `X-Request-ID`.

## Основная проверка

В запросах коллекции уже заданы заголовки `Content-Type: application/json` и `X-Telegram-User-Id: {{telegram_user_id}}`. Авторизация использует `{{service_token}}`.

`telegram_user_id` используется во вкладке **Headers**, даже если его нет в JSON Body. Заполни его во вкладке **Variables** самой коллекции: это числовой Telegram ID, соответствующий `subject_id` в PostgreSQL. Если переменная пустая или заголовок отключён, запрос вернёт HTTP 400:

```json
{
  "error_code": "invalid_request",
  "message": "X-Telegram-User-Id header is required",
  "request_id": "5dd9cb63-5c64-4c27-b9f8-c9d9be3bb67e"
}
```

Для нечислового ID сообщение объясняет требуемый формат; для ошибок полей тела указывается JSON-путь и ограничение. Если ошибок несколько, исправляй их последовательно по ответам. Запрос «12 — отсутствует Telegram-заголовок» проверяет этот сценарий с корректным телом и сервисным токеном.

```json
{
  "request_id": "{{$guid}}",
  "subject": {"type": "user", "id": "{{subject_id}}"},
  "action": {"name": "update", "attributes": {"field_mask": ["text", "title"]}},
  "resource": {"type": "note", "id": "{{note_id}}"},
  "options": {"include_reasons": true, "consistency": "higher_consistency"}
}
```

Для owner/editor ожидается HTTP 200, `result: "allow"`, `allow: true`, причины `allowed_via_fga_relation` и `allowed_fields`. В `meta.resource` должен быть UUID проверяемой заметки, в `authorization_model_id` — закреплённая модель. Для viewer та же операция даёт HTTP 200 с deny; для него `read` должен быть разрешён.

При смене пользователя меняй одновременно `telegram_user_id` и `subject_id`. Сервисный токен может оставаться тем же: это токен bot-клиента, а не Telegram-пользователя.

## Что проверить

| Изменение запроса/условий | Ожидание |
|---|---|
| У пользователя есть update, маска `text,title,timestamp` | 200 deny, `forbidden_fields`; вся операция запрещена |
| Reminder, update, маска `title,remind_at` | 200 allow при наличии права update |
| Reminder, stop без `field_mask` | 200 allow для editor/владельца; напоминание фактически не меняется |
| Owner personal запрашивает space:delete | 200 deny |
| Owner personal запрашивает delete записи внутри personal | 200 allow; для этой проверки подставь UUID личной записи |
| Owner/admin shared меняет участие owner | 200 deny, `protected_space_owner` |
| Member:add/edit с `role: "owner"` | 400 `invalid_request` |
| Member:add/edit без role или member:remove с role | 400 `invalid_request` |
| Update с пустой/отсутствующей маской; read с маской | 400 `invalid_request` |
| `include_reasons: false` | `reasons: []` при завершённом решении |
| Корректный заголовок, другой `subject.id` | 403 `subject_mismatch` |
| Диагностический `subject.attributes.telegram_user_id` не равен заголовку | 400 `invalid_request` |
| Неизвестное поле, `null`, невалидный UUID | 400 `invalid_request` |
| Нет токена или токен просрочен | 401 `unauthorized` |
| Валидный токен, но клиент неактивен или scope запрещён | 403 `forbidden` |
| Telegram ID не зарегистрирован | 404 `user_not_found` |
| Корректный, но отсутствующий в FGA UUID записи | 200 deny |
| Два запроса с одним явно заданным `request_id` | Новый `decision_id` каждый раз; решение вычисляется заново |

При `include_reasons: true` проверка отношений объясняет, что именно проверено: `OpenFGA allowed relation "can_update" on note:<UUID> for user:<UUID>` или аналогичное сообщение с `denied`. Для создания записи и управления участниками проверяется родительский `space`. Сообщение не определяет роль пользователя или путь наследования права. Проверка полей перечисляет только имена из присланной маски: `Allowed fields for note:update: text, title`; при запрете — `Forbidden fields for note:update: timestamp`. Имена отсортированы, дубли убраны.

Обычный deny — HTTP 200. Ошибки 4xx/5xx содержат `error_code`, безопасное `message` и, когда разобран, `request_id`; в них нет `allow`, `result`, `decision_id`.

Чтобы вручную проверить 503, сначала запусти сервис с доступным локальным FGA, затем сделай FGA недоступным и отправь check существующей записи. Ожидается `503 openfga_unavailable`; сетевое ожидание ограничено `openfga.request_timeout`. Если FGA недоступен уже при запуске, сервис может не стартовать — это другая проверка. У member два последовательных FGA-check, поэтому таймаут одного обращения не является общим SLA HTTP-запроса.

## Если нужно подготовить минимальные отношения вручную

Используй локальные тестовые данные и существующий `POST /api/v0/resources/update`. Для нового shared сначала получи allow от check с `action.name=create`, `resource.type=space`, новым UUID ресурса; скопируй `decision_id` в следующий запрос. Владелец должен совпадать с пользователем из заголовка.

```json
{
  "request_id": "{{$guid}}",
  "idempotency_key": "{{$guid}}",
  "decision_id": "UUID из ответа check",
  "resource": {"type": "space", "id": "{{shared_space_id}}", "attributes": {"space_type": "shared"}},
  "relations": {"owner": {"type": "user", "id": "{{subject_id}}"}},
  "operation": "create",
  "change_type": "resource_added",
  "context": {"source_service": "postman", "event_type": "SPACE_CREATED"}
}
```

Для note сначала проверь `note:create` с будущим `note_id` и `resource.attributes.space_id={{shared_space_id}}`. Затем синхронизируй создание:

```json
{
  "request_id": "{{$guid}}",
  "idempotency_key": "{{$guid}}",
  "decision_id": "UUID из ответа note:create check",
  "resource": {"type": "note", "id": "{{note_id}}"},
  "relations": {
    "owner": {"type": "user", "id": "{{subject_id}}"},
    "parent": {"type": "space", "id": "{{shared_space_id}}"}
  },
  "operation": "create",
  "change_type": "resource_added",
  "context": {"source_service": "postman", "event_type": "NOTE_CREATED"}
}
```

Для reminder замени тип/ID и event_type на `reminder`, `reminder_id`, `REMINDER_CREATED`. Для personal используй `space_type: "personal"` и отдельные UUID пространства/его записей. Перед созданием заполни переменные новыми UUID. В `written_tuples` при создании пространства проверь общую отметку `space:<id>#created@user:*`, owner и нужный тип. Для personal там же должна быть отметка владельца `has_personal_space`. Второй синхронный или параллельный create того же UUID получит 409. `decision_id` пока служит корреляции, идемпотентность UpdateResource и универсальная синхронизация изменений участников реализуются отдельно; роли editor/viewer подготавливаются внешними тестовыми фикстурами, а не обещанным здесь member-write.

## Что означает защита от второго родителя

В нормальных данных запись относится к одному пространству. OpenFGA, однако, допускает одновременно `note:N#space@space:personal-A` и `note:N#space@space:shared-B`: модель сама не проверяет количество таких связей. Обычно разрешения от родителей объединяются, и участник B мог бы получить доступ к личной записи A.

`foreign_personal` и `blocked_by_personal` вычитают доступ через чужой personal из публичных прав записи. Даже разрешение через shared-B не открывает её пользователю B. Это защита приватности при ошибочных/конкурирующих tuples, а не поддержка нескольких пространств у одной записи и не исправление повреждённых данных. Для ручной регрессии подготовь такие tuples во внешней тестовой фикстуре и проверь чужим пользователем read/update/delete, а у reminder также stop: везде должен быть deny.

Матрица публичных решений проверяется в pytests на реальном OpenFGA. Интеграционные тесты CLI и миграции графа выполняются в auth-service.
