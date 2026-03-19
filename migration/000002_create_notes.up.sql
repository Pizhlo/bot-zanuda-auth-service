DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_type WHERE typname = 'note_visibility_type'
  ) THEN
    CREATE TYPE note_visibility_type AS ENUM ('SPACE', 'PRIVATE_TO_AUTHOR', 'CUSTOM');
  END IF;
END $$;

-- SPACE - обычная заметка; подчиняется правам роли. Видна всем участникам, у кого по роли есть право читать заметки (NOTE_VIEW_ALL/NOTE_VIEW_OWN),

-- PRIVATE_TO_AUTHOR - Заметка привязана к конкретному пользователю, даже если она создана в shared‑пространстве. 
-- Видна только автору (и, возможно, системным/админ‑ролям), остальные участники пространства её не увидят, даже если у них NOTE_VIEW_ALL.
-- Это аналог «личной заметки внутри общего пространства».

-- CUSTOM - Включается, когда нужны индивидуальные исключения. Для такой заметки существуют записи в note_acl:
-- можно явным DENY спрятать заметку от конкретного участника;
-- можно ALLOW + can_read / can_edit выдать отдельному человеку права, отличающиеся от роли.
-- Без записей в note_acl для CUSTOM ты сама выбираешь дефолт — чаще всего безопасно делать deny по умолчанию.

CREATE SCHEMA IF NOT EXISTS notes;

-- это не основная таблица с заметками, здесь содержится только информация, 
-- необходимая для реализации политик
CREATE TABLE IF NOT EXISTS notes.notes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    space_id        uuid NOT NULL REFERENCES spaces.spaces(id) ON DELETE CASCADE,
    author_id       uuid NOT NULL REFERENCES users.users(id) ON DELETE CASCADE,

    visibility_type note_visibility_type NOT NULL DEFAULT 'SPACE',

    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ
);

-- ACL для заметок (исключения: скрыть/дать доп.права конкретному юзеру)
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_type WHERE typname = 'note_acl_access'
  ) THEN
    CREATE TYPE note_acl_access AS ENUM ('ALLOW', 'DENY');
  END IF;
END $$;

CREATE TABLE IF NOT EXISTS notes.note_acl (
    id          BIGSERIAL PRIMARY KEY,
    note_id     uuid NOT NULL REFERENCES notes.notes(id) ON DELETE CASCADE,
    user_id     uuid NOT NULL REFERENCES users.users(id) ON DELETE CASCADE,

    access      note_acl_access NOT NULL,  -- ALLOW / DENY
    can_read    BOOLEAN,                   -- NULL = не переопределяем
    can_edit    BOOLEAN,                   -- NULL = не переопределяем

    UNIQUE (note_id, user_id)
);

ALTER TABLE notes.note_acl
  ADD CONSTRAINT note_acl_deny_no_flags
  CHECK (
    NOT (access = 'DENY' AND (COALESCE(can_read, false) = true OR COALESCE(can_edit, false) = true))
  );


-- Индексы для батч-проверок
CREATE INDEX IF NOT EXISTS idx_note_space ON notes.notes(space_id);
