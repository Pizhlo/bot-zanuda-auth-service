# Проверка одного действия

`POST /v0/decisions/check` проверяет права без изменения бизнес-данных, регистрации пользователей или записи отношений. Канонические правила: [specs/auth-service/spec.md](../specs/auth-service/spec.md). Старый черновой `/api/v0/resources/decisions/check` удалён. Остальные маршруты сохраняют свои адреса и форматы ответов.

Заголовки: `Authorization: Bearer <service token>` и `X-Telegram-User-Id: 123456789`. JWT проверяется до обработки личности. Нужен активный сервисный клиент со scope `bot`; каждый scope из списка токена должен быть разрешён клиенту. Заголовок пользователя передаётся только доверенным маршрутом bot → web-server → auth-service. Telegram ID — положительная десятичная строка без знака, пробелов и ведущих нулей, в диапазоне int64.

## Поля запроса

| Поле | Обязательно | Ограничения |
|---|---|---|
| `request_id` | Да | Ненулевой UUID; корреляция, без replay |
| `subject` | Да | Объект с `type` и `id` |
| `subject.type` | Да | Только `user` |
| `subject.id` | Да | Ненулевой внутренний UUID, совпадающий с пользователем из доверенного заголовка |
| `subject.attributes.telegram_user_id` | Нет | Строка; совпадает с заголовком |
| `action.name` | Да | Поддерживаемая комбинация из матрицы ниже |
| `action.attributes.field_mask` | Для `update` | Непустой массив непустых имён; для остальных действий недопустим |
| `action.attributes.role` | Для `member:add/edit` | `viewer`, `editor`, `admin`; для остальных действий недопустим |
| `resource.type` | Да | `note`, `reminder`, `space`, `member` |
| `resource.id` | Да | Ненулевой UUID, включая ещё не созданный ресурс; для member — целевой пользователь |
| `resource.attributes.space_id` | Для note/reminder create и всех member | Ненулевой UUID родителя; у существующих записей необязателен и не переопределяет родителя OpenFGA; для space запрещён |
| `context.service/client/trace_id` | Нет | Строки для диагностики, не дают прав |
| `context.ip` | Нет | Корректный IPv4/IPv6 |
| `context.time` | Нет | RFC3339; не заменяет серверное время аудита |
| `options.include_reasons` | Нет | Boolean, default `false` |
| `options.consistency` | Нет | `higher_consistency` (default) или `minimize_latency` |

Необязательные объекты `attributes`, `context`, `options` можно опустить или передать пустыми, кроме условно обязательных атрибутов. Неизвестные поля, `null`, неправильные типы и неподдерживаемые комбинации дают 400. `subject.relation`, `is_admin`, отдельный body `user_id` отсутствуют. Максимальный размер тела — 1 MiB.

| Действия | Проверка |
|---|---|
| note:create, reminder:create | Право `can_create_note/reminder` на родительском space |
| note:read/update/delete | `can_view/can_update/can_delete` на самой note |
| reminder:read/update/delete/stop | `can_view/can_update/can_delete/can_stop` на самом reminder |
| space:create | Разрешено каждому зарегистрированному доверенному пользователю; будущий owner — этот пользователь; FGA не вызывается |
| space:read/update/delete | `can_view/can_manage_space/can_delete` |
| member:add/edit/remove | `can_manage_members` на space, затем отдельный запрет менять участие его owner |

Personal — встроенное пространство пользователя. Только его owner имеет доступ к нему и записям. Нельзя удалить само пространство или управлять участниками, даже владельцу. Владельцу доступны изменения/удаление записей. У shared удалить пространство или изменить имя может только owner; admin управляет участниками, в том числе своей ролью, но не owner.

## Политика полей

После разрешения OpenFGA реальный Casbin проверяет **всю** маску:

| Ресурс | Разрешённые поля |
|---|---|
| note | `text`, `title` |
| reminder | `remind_at`, `title` |
| space | `name` |

Списки одинаковы для всех пользователей с правом на объект. Любое техническое или неизвестное непустое имя, например `created_at`, `updated_at`, `timestamp`, запрещает всю операцию с HTTP 200. Пустая/неверно типизированная маска — 400. Порядок и дубли не влияют на решение; список запрещённых полей отсортирован и не содержит дублей.

Модель и правила встроены из `internal/service/enforcer/fields/`; нет API редактирования, hot reload, БД ролей или отдельного Casbin role source. Версия `casbin:sha256:<hex>` вычисляется по фактической модели и отсортированным правилам. Изменение требует новой сборки. Ошибка загрузки блокирует запуск. Поля reminder пока предварительные и могут измениться вместе с будущей моделью данных.

## Примеры и ответ

Полные JSON: [request allow](examples/decisions/request-allow.json), [response allow](examples/decisions/response-allow.json), [request deny](examples/decisions/request-deny.json), [response deny](examples/decisions/response-deny.json). У deny маска действительно содержит `timestamp`. UUID ресурса в `meta.resource` совпадает с запросом. Значения версий/latency в ответах иллюстративны; runtime сообщает фактические версии.

Завершённый allow **и** deny возвращаются с HTTP 200. Ответ содержит новый UUID `decision_id`, исходный `request_id`, строку `result`, согласованный boolean `allow`, массив `reasons` и `meta`. При `include_reasons=false` массив всегда `[]`. При true коды: `allowed_via_fga_relation`, `allowed_fields`, `allowed_for_registered_user`, `denied_by_fga_relation`, `forbidden_fields`, `protected_space_owner`. Сообщение не приписывает boolean-ответу FGA недоказанный путь наследования роли. При отказе объясняется только этап отказа.

`meta` содержит доверенный `subject` (`user:UUID`), запрошенные `action` и `resource` (`type:UUID`), `policy_version`, закреплённый `authorization_model_id`, неотрицательный `latency_ms`. Проверяемый родитель create/member записывается отдельно в аудит. Для space:create ID модели сообщается без вызова FGA.

## Ошибки

| HTTP | error_code | Смысл |
|---|---|---|
| 400 | `invalid_request` | JSON, поля, UUID, заголовок, маска или комбинация неверны |
| 401 | `unauthorized` | Нет валидного JWT или обязательных claims |
| 403 | `forbidden` | Клиент отсутствует/неактивен, нет bot или scope не разрешён |
| 403 | `subject_mismatch` | Личность тела не совпадает с доверенной |
| 404 | `user_not_found` | Telegram-пользователь ещё не зарегистрирован |
| 503 | `identity_store_unavailable` | Ошибка хранилища клиентов/пользователей |
| 503 | `openfga_unavailable` | Проверка отношений завершилась ошибкой |
| 503 | `policy_unavailable` | Ошибка вычисления Casbin |
| 500 | `internal_error` | Непредвиденная ошибка; детали не раскрываются |

Ошибки содержат только `error_code`, безопасное `message`, `request_id`, если удалось разобрать ненулевой UUID. Пример: [400](examples/decisions/error-invalid.json). В ошибках отсутствуют `allow`, `result` и `decision_id`. Ошибка зависимости не превращается в обычный deny.

## Аудит и повтор

Каждый завершённый check записывает локальное структурированное событие kind `domain`, level `info`: caller, доверенный subject, action, запрошенный resource, checked object/relation, result, серверное время, request/decision ID и версии. Для member добавляются target_user_id/role. Для space:create checked object/relation равны null. Секреты и JWT claims не записываются. RabbitMQ отправляется по существующим фильтрам аудита, с лимитом ожидания 1 секунда; ошибка доставки не меняет ответ, локальная запись остаётся обязательной. При истечении лимита закрывается зависший AMQP transport, поскольку SDK не прерывает начатую отправку по context. Последующие публикации в закрытое соединение завершаются ошибкой до штатного перезапуска клиента/сервиса; это не меняет решения.

Одинаковый request_id снова проверяет актуальные отношения, получает новый decision_id и новое событие. Сервис не кеширует решения; minimize_latency разрешает кеш OpenFGA и передаётся обоим checks участника. decision_id — корреляция, не одноразовый пропуск. Между check и последующим write нет атомарности или гарантии сохранения прав. Идемпотентность UpdateResource — отдельная задача.
