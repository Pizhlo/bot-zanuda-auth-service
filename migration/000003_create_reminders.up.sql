CREATE SCHEMA IF NOT EXISTS reminders;

-- это тоже не основная таблица с напоминаниями.
-- основная таблица - у сервиса напоминаний.
-- здесь только информация для сервиса авторизации.
CREATE TABLE IF NOT EXISTS reminders.reminders (
    id          BIGSERIAL PRIMARY KEY,
    space_id    BIGINT NOT NULL REFERENCES spaces.spaces(id) ON DELETE CASCADE,
    author_id   BIGINT NOT NULL REFERENCES users.users(id) ON DELETE CASCADE,

    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Кому летит напоминание и какие у него перс. права
CREATE TABLE IF NOT EXISTS reminders.reminder_recipient (
    reminder_id BIGINT NOT NULL REFERENCES reminders.reminders(id) ON DELETE CASCADE,
    user_id     BIGINT NOT NULL REFERENCES users.users(id) ON DELETE CASCADE,

    can_stop    BOOLEAN,        -- NULL = по роли, TRUE/FALSE = override

    PRIMARY KEY (reminder_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_reminder_space ON reminders.reminders(space_id);
CREATE INDEX IF NOT EXISTS idx_reminder_recipient_user ON reminders.reminder_recipient(user_id);
