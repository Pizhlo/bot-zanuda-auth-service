# Проверка одного действия

`POST /api/v0/decisions/check` проверяет права без изменения бизнес-данных, регистрации пользователей или записи отношений. Канонические правила: [specs/auth-service/spec.md](../specs/auth-service/spec.md). Старый черновой `/api/v0/resources/decisions/check` удалён. Остальные маршруты сохраняют свои адреса и форматы ответов.

Заголовки: `Authorization: Bearer <service token>` и `X-Telegram-User-Id: 123456789`. JWT проверяется до обработки личности. Нужен активный сервисный клиент со scope `bot`; каждый scope из списка токена должен быть разрешён клиенту. Заголовок пользователя передаётся только доверенным маршрутом bot → web-server → auth-service. Telegram ID — положительная десятичная строка без знака, пробелов и ведущих нулей, в диапазоне int64.

Текущий маршрут поддерживает вызовы от имени пользователя Telegram. Для будущих прямых запросов админ-сервиса потребуется проверять его сервисный scope и независимо подтверждать личность администратора. Поле `subject.id` в теле сверяется с доверенной личностью.

## Поля запроса

Имена полей на каждом уровне JSON должны точно совпадать с таблицей. Каждое поле объекта можно передать один раз. Повтор ключа или иной регистр приводит к HTTP 400 `invalid_request`, в том числе при одинаковых значениях. Сервис отклоняет такой запрос до проверки прав и сохранения решения. Если сам `request_id` повторён или передан в другом регистре, тело ошибки не содержит идентификатор запроса. Корреляция в заголовке ответа и аудите в этом случае берётся из HTTP-запроса или генерируется сервисом.

| Поле | Обязательно | Ограничения |
|---|---|---|
| `request_id` | Да | Ненулевой UUID; корреляция, без replay |
| `subject` | Да | Объект с `type` и `id` |
| `subject.type` | Да | Только `user` |
| `subject.id` | Да | Ненулевой внутренний UUID, совпадающий с пользователем из доверенного заголовка |
| `subject.attributes.telegram_user_id` | Нет | Строка; совпадает с заголовком, иначе 400 |
| `action.name` | Да | Поддерживаемая комбинация из матрицы ниже |
| `action.attributes.field_mask` | Для `update` | Непустой массив непустых имён; для остальных действий недопустим |
| `action.attributes.role` | Для `member:add/edit` | `viewer`, `editor`, `admin`; для остальных действий недопустим |
| `resource.type` | Да | `note`, `reminder`, `space`, `member` |
| `resource.id` | Да | Ненулевой UUID, включая ещё не созданный ресурс; для member — целевой пользователь |
| `resource.attributes.space_id` | Для note/reminder create и всех member | Ненулевой UUID родителя; у существующих записей необязателен и не переопределяет родителя OpenFGA; для space запрещён |
| `resource.attributes.space_type` | Только для space:create, обязательно | `shared` → allow, `personal` → allow, если у пользователя ещё нет личного пространства. Отсутствующий или неизвестный тип даёт 400 |
| `context.service/client/trace_id` | Нет | Строки для диагностики, не дают прав |
| `context.ip` | Нет | Корректный IPv4/IPv6 |
| `context.time` | Нет | RFC3339; не заменяет серверное время аудита |
| `options.include_reasons` | Нет | Boolean, default `false` |
| `options.consistency` | Нет | `higher_consistency` (default) или `minimize_latency` |

Необязательные объекты `attributes`, `context`, `options` можно опустить или передать пустыми, кроме условно обязательных атрибутов. Неизвестные поля, `null`, неправильные типы и неподдерживаемые комбинации дают 400. `subject.relation`, `is_admin`, отдельный body `user_id` отсутствуют.

| Действия | Проверка |
|---|---|
| note:create, reminder:create | Право `can_create_note/reminder` на родительском space |
| note:read/update/delete | `can_view/can_update/can_delete` на самой note |
| reminder:read/update/delete/stop | `can_view/can_update/can_delete/can_stop` на самом reminder |
| space:create | Будущий владелец — подтверждённый пользователь. Для `shared` сервис читает закреплённую модель OpenFGA и возвращает allow зарегистрированному пользователю. Ошибка чтения модели даёт 503 `openfga_unavailable`. Для `personal` сервис проверяет `has_personal_space` на `user:<subject_id>` с `higher_consistency`: есть отметка → deny, нет → allow |
| space:read/update/delete | `can_view/can_manage_space/can_delete` |
| member:add/edit/remove | `can_manage_members` на space, затем отдельный запрет менять участие его owner |

Если у записи ошибочно сохранено несколько parent-связей, наличие чужого personal запрещает доступ даже при разрешающем shared-родителе. Уникальность родителя самим OpenFGA не гарантируется.

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

Полные JSON: [request allow](examples/decisions/request-allow.json), [response allow](examples/decisions/response-allow.json), [request deny](examples/decisions/request-deny.json), [response deny](examples/decisions/response-deny.json). `request-*.json` содержит только входные данные. `include_reasons: true` просит объяснить решение; сами причины появляются в парном `response-*.json`. Например, [request-deny.json](examples/decisions/request-deny.json) → [response-deny.json](examples/decisions/response-deny.json), где `reasons` содержит `forbidden_fields`. У deny маска действительно содержит `timestamp`. UUID ресурса в `meta.resource` совпадает с запросом. Значения версий/latency в ответах иллюстративны; runtime сообщает фактические версии.

Завершённый allow **и** deny возвращаются с HTTP 200. Ответ содержит новый UUID `decision_id`, исходный `request_id`, строку `result`, согласованный boolean `allow`, массив `reasons` и `meta`. При `include_reasons=false` массив всегда `[]`. При true коды: `allowed_via_fga_relation`, `allowed_fields`, `allowed_for_registered_user`, `first_personal_space`, `personal_space_already_exists`, `denied_by_fga_relation`, `forbidden_fields`, `protected_space_owner`. Причины проверки отношений называют доверенного пользователя, проверенные отношение и объект: например, `OpenFGA allowed relation "can_update" on note:<UUID> for user:<UUID>`. Для create записи и member это родительский `space`, а не `meta.resource`. При deny сообщение называет ту же фактическую проверку. Boolean-ответ FGA не раскрывает, через какую роль или цепочку наследования получено право.

`allowed_fields` перечисляет проверенные поля из запроса, сортируя их и убирая дубли: `Allowed fields for note:update: text, title`. Это не полный список доступных полей политики. `forbidden_fields` аналогично называет тип ресурса, действие и запрещённые поля: `Forbidden fields for note:update: timestamp`. `protected_space_owner` указывает целевого пользователя и пространство, участие владельца которого нельзя менять. `allowed_for_registered_user` указывает зарегистрированного пользователя и UUID будущего общего пространства. Для `personal` причины `first_personal_space` и `personal_space_already_exists` сообщают, найдена ли отметка владельца. Проверка оставляет её без изменений. При создании `resources/update` записывает владельца, тип и отметку одной транзакцией. Если две проверки вернули allow одновременно, вторая запись того же владельца получит конфликт. При отказе объясняется только этап отказа.

`meta` содержит доверенный `subject` (`user:UUID`), запрошенные `action` и `resource` (`type:UUID`), `policy_version`, закреплённый `authorization_model_id`, неотрицательный `latency_ms`. Проверяемый родитель create/member записывается отдельно в аудит. Для shared space:create сервис подтверждает доступность закреплённой модели её чтением.

## Ошибки

| HTTP | error_code | Смысл |
|---|---|---|
| 400 | `invalid_request` | JSON, поля, UUID, заголовок, маска или комбинация неверны |
| 401 | `unauthorized` | Нет валидного JWT или обязательных claims |
| 403 | `forbidden` | Клиент отсутствует/неактивен, нет bot или scope не разрешён |
| 403 | `subject_mismatch` | Личность тела не совпадает с доверенной |
| 404 | `user_not_found` | Telegram-пользователь ещё не зарегистрирован |
| 503 | `decision_store_unavailable` | PostgreSQL не подтвердил запись решения |
| 503 | `identity_store_unavailable` | Ошибка хранилища клиентов/пользователей |
| 503 | `openfga_unavailable` | Проверка отношений завершилась ошибкой |
| 503 | `policy_unavailable` | Ошибка вычисления Casbin |
| 500 | `internal_error` | Непредвиденная ошибка; детали не раскрываются |

Ошибки содержат только `error_code`, безопасное `message`, `request_id`, если удалось разобрать ненулевой UUID. Пример: [400](examples/decisions/error-invalid.json). В ошибках отсутствуют `allow`, `result` и `decision_id`. Ошибка зависимости не превращается в обычный deny. Каждый CheckRelation ограничен `openfga.request_timeout` (по умолчанию 5s) и более ранним deadline вызывающего кода; у member два последовательных checks. Это бюджет отдельного обращения к FGA, а не общий SLA HTTP-запроса.

Для `400 invalid_request` поле `message` называет проблемный заголовок или JSON-путь и нарушенное требование. Например, пустой/отсутствующий заголовок даёт `X-Telegram-User-Id header is required`, а update без маски — `action.attributes.field_mask must be nonempty for update`. Неверный формат Telegram ID и несовпадение его диагностической копии в теле имеют отдельные объяснения. Ошибки разбора UUID, типов и `null` также указывают JSON-путь: `resource.id must be a nonzero UUID`, `options.include_reasons must be a boolean`, `action.attributes.field_mask[1] must be a string`. Пустая или не подставленная переменная Postman `target_user_id` в member-запросе относится к `resource.id`. Неизвестный ключ описывается через известный родительский объект, например `action.attributes contains an unknown field`, без повторения присланного имени.

Ответ сообщает одну ошибку за раз; ошибки чтения, размера или синтаксиса JSON относятся ко всему `body`. Для синтаксической ошибки, когда парсер определил позицию, указывается смещение в байтах: `body contains invalid JSON at byte 42`. В сообщениях `invalid_request` не повторяются присланные значения, внутренние обёртки или ошибки зависимостей.

При отказе в аутентификации сервис оставляет тело непрочитанным. Передай один ненулевой UUID в заголовке `X-Request-ID`, чтобы получить его в поле `request_id` ошибки. Если заголовок отсутствует, повторяется, содержит нулевой или некорректный UUID, сервис генерирует ID и опускает поле `request_id` в JSON ошибки. В обоих случаях ID в заголовке ответа `X-Request-ID` совпадает с ID предупреждения и итогового события аудита. После успешной аутентификации решение использует обязательный `request_id` из JSON.

## Сохранение решений

Каждый HTTP 200 означает, что PostgreSQL подтвердил вставку в `auth.decisions`. Миграция 000006 применяется при запуске. Найти решение можно по `decision_id` из ответа. Колонка `request_id` позволяет найти все повторы запроса. Для каждой попытки, завершившейся решением, создаётся отдельная строка. Таблица содержит результат, идентификаторы клиента и пользователя, действие с маской полей или ролью, ресурс, присланный space_id, проверенную пару объект/отношение, версии FGA и Casbin, причины решения и серверное время. Причины доступны в БД и при `include_reasons=false`.

```sql
SELECT decision_id, request_id, result, action, resource_type, resource_id, created_at
FROM auth.decisions
ORDER BY created_at DESC
LIMIT 10;
```

## Аудит и повтор

Каждая попытка проверки создаёт итоговое событие `operation=check_decision` с `request_id`. При ошибке в событии указаны `status=failed`, этап и безопасные `error_code` и `message`. До вычисления решения поля `decision_id` и `decision` отсутствуют. При ошибке INSERT событие сохраняет вычисленное `decision` (allow или deny), сформированный `decision_id`, `policy_version` и `authorization_model_id`. Этап — `save_decision`, код ошибки — `decision_store_unavailable`. Клиент получает HTTP 503 с кодом ошибки и `request_id`. По `decision_id` из аудита можно проверить в PostgreSQL, сохранилась ли запись. Ошибка записи HTTP-ответа также попадает в событие, включая случай, когда уже сохранено решение или возникла другая ошибка.

Если локальный журнал не принимает начальное событие, обработка останавливается до проверки токена. Ответ решения содержит HTTP 500 и `error_code=internal_error`. Предупреждение о сбое локальной записи содержит `local_write_failed=true`. Если writer отказал уже при завершении, аудитор всё равно пробует отправить событие по правилам RabbitMQ. Отказ конечной записи не отменяет сохранённое решение.

Каждый завершённый check записывает локальное структурированное событие kind `domain`, level `info`: caller, доверенный subject, action, запрошенный resource, checked object/relation, result, серверное время, request/decision ID и версии. Для member добавляются target_user_id/role. Если OpenFGA ответил на дополнительную проверку владельца, `target_is_owner` содержит её результат (`true` или `false`). При пропуске или ошибке этой проверки поле отсутствует. Для space:create сохраняется запрошенный space_type. Для shared поля checked object/relation пусты, для personal они содержат `user:<subject_id>` и `has_personal_space`. Секреты и JWT claims не записываются. RabbitMQ отправляется по существующим фильтрам аудита, с лимитом `audit.delivery_timeout` (по умолчанию 1s); ошибка доставки не меняет ответ, локальная запись остаётся обязательной. При истечении лимита закрывается зависший AMQP transport, поскольку SDK не прерывает начатую отправку по context. Последующие публикации в закрытое соединение завершаются ошибкой до штатного перезапуска клиента/сервиса; это не меняет решения.

При создании заметки или напоминания решение разрешает действие на родительском пространстве. Уникальность UUID при записи гарантирует сервис, хранящий объект. После успешного создания он синхронизирует права через `resources/update`; при конфликте UUID событие создания не отправляется.

Два вызова `space:create` могут получить allow для одного будущего UUID. При синхронизации `resource_added` OpenFGA записывает одну и ту же отметку `space:ID#created@user:*` вместе с остальными отношениями. Один запрос проходит, второй получает HTTP 409. Все отношения победившего запроса записываются вместе.

Одинаковый request_id снова проверяет актуальные отношения, получает новый decision_id и новое событие. Сервис не кеширует решения; minimize_latency разрешает кеш OpenFGA и передаётся обоим checks участника. decision_id — корреляция, не одноразовый пропуск. Между check и последующим write нет атомарности или гарантии сохранения прав. Идемпотентность UpdateResource — отдельная задача.

## Ручная проверка и границы слоёв

[Инструкция Postman и коллекция запросов](postman/README.md) содержат подготовку, логин, положительные и отрицательные сценарии. DTO проверяет структуру и форматы; допустимость операции, условные атрибуты и назначаемую роль проверяет сервис решений до обращений к внешним системам. В Go остаются unit-тесты с локальными заглушками; проверки настоящих OpenFGA/RabbitMQ и сквозные сценарии пишутся на Python в отдельном репозитории.
