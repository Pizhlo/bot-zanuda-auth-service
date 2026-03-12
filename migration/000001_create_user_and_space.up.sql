-- https://www.perplexity.ai/search/ia-pytaius-sproektirovat-siste-AJFn5.BSSPi.XHuALnU5KQ

CREATE EXTENSION IF NOT EXISTS pgcrypto;

ALTER TABLE public.schema_migrations 
  ADD COLUMN IF NOT EXISTS created TIMESTAMP NOT NULL DEFAULT now();

CREATE SCHEMA  IF NOT EXISTS users;

-- Пользователь
CREATE TABLE IF NOT EXISTS users.telegram (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tg_id           BIGINT UNIQUE NOT NULL,

    global_invite_policy VARCHAR(32) NOT NULL DEFAULT 'ALLOW_ALL',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_type WHERE typname = 'space_type'
  ) THEN
    CREATE TYPE space_type AS ENUM ('PERSONAL', 'SHARED');
  END IF;
END $$;

-- PERSONAL - личное пространство пользователя. Видно только ему. В него нельзя никого приглашать (пользователь его даже не видит и не знает о нем).
-- Тут хранятся личные записи пользователя.
-- SHARED - совместное пространство. Это пространство, создаваемое пользователями. Сюда можно приглашать других пользователей.
-- Здесь хранятся записи всех, кто сюда что-либо писал (не только админа, если такое разрешают права).

CREATE SCHEMA IF NOT EXISTS spaces;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_type WHERE typname = 'space_role_code'
  ) THEN
   CREATE TYPE space_role_code AS ENUM ('OWNER', 'ADMIN', 'EDITOR', 'VIEWER', 'CUSTOM');
  END IF;
END $$;
-- OWNER - может все.
-- ADMIN - все, кроме назначения ролей.
-- EDITOR - только читает и пишет, но не управляет участниками.

-- Пространство
CREATE TABLE IF NOT EXISTS spaces.spaces (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type                space_type NOT NULL DEFAULT 'PERSONAL',
    owner_id            uuid NOT NULL REFERENCES users.telegram(id) ON DELETE CASCADE,
    default_participant_role VARCHAR(64) NOT NULL DEFAULT 'EDITOR',

    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Роль в пространстве (глобальный шаблон)
CREATE TABLE IF NOT EXISTS spaces.space_role (
    id          BIGSERIAL PRIMARY KEY,
    code        space_role_code NOT NULL,       -- OWNER / ADMIN / EDITOR / VIEWER / CUSTOM
    name        TEXT NOT NULL, -- человекочитаемое название: владелец, читатель, админ, и т.д.
    UNIQUE (code)
);

-- Права ролей в пространстве
CREATE TABLE IF NOT EXISTS spaces.space_role_permission (
    id          BIGSERIAL PRIMARY KEY,
    role_id     BIGINT NOT NULL REFERENCES spaces.space_role(id) ON DELETE CASCADE,
    permission  VARCHAR(64) NOT NULL,       -- NOTE_VIEW_ALL, NOTE_EDIT_ANY, REMINDER_CREATE и т.п.

    UNIQUE (role_id, permission)
);

-- Роли
INSERT INTO spaces.space_role (code, name) VALUES
  ('OWNER',  'Владелец'),
  ('ADMIN',  'Администратор'),
  ('EDITOR', 'Редактор'),
  ('VIEWER', 'Читатель')
ON CONFLICT (code) DO NOTHING;

-- OWNER (Владелец):
  -- Полный контроль над пространством.
  -- Может менять любые настройки пространства (название, политика видимости, дефолтная роль участников).
  -- Может управлять ролями участников (повышать/понижать, кикать).
  -- Может приглашать и удалять участников, менять им роли.
  -- Имеет полный доступ к контенту:
    -- видит все заметки и напоминания в пространстве, включая приватные и CUSTOM;
    -- может создавать/редактировать/удалять любые заметки;
    -- может создавать/редактировать/удалять любые напоминания, останавливать их для любых участников.
    -- Может настраивать объектные политики (ACL): скрывать заметки от участников, давать точечные права.
  -- Привязываем ID ролей к переменным

-- ADMIN (Администратор)
  -- Почти полный контроль над пространством, но без «владельческих» операций.
  -- Может:
    -- менять часть настроек пространства (например, описание, картинку, дефолтную роль), если ты это разрешишь;
    -- приглашать и удалять участников (кроме владельца);
    -- менять роли участников, но не может передать/забрать роль владельца.
  -- Доступ к контенту:
    -- видит все заметки и напоминания в пространстве;
    -- может создавать/редактировать/удалять любые заметки (кроме, если ты введёшь жёстко приватные OWNER‑заметки);
    -- может создавать/редактировать/удалять любые напоминания, останавливать их для любых участников.
    -- Может управлять видимостью заметок (настраивать ACL), но в пределах политик, которые задаёт владелец (по желанию).

-- EDITOR (Редактор)
  -- Рабочая роль для активных участников.
  -- Доступ к контенту:
    -- видит все заметки и напоминания в пространстве (кроме скрытых от него через ACL или сугубо приватных);
    -- может создавать новые заметки;
    -- может редактировать свои заметки; по желанию — и чужие (это определяется набором разрешений NOTE_EDIT_OWN / NOTE_EDIT_ANY);
    -- может удалять свои заметки; обычно не может удалять чужие.
    -- может создавать напоминания и управлять своими напоминаниями (редактировать / останавливать);
    -- может видеть напоминания, назначенные ему, и по настройкам — напоминания в пространстве в целом.
  -- Не управляет участниками и настройками пространства (нет прав на инвайты/кики/смену ролей).

-- VIEWER (Читатель)
  -- Роль только для чтения.
  -- Может:
    -- видеть все заметки, которые ему не скрыли ACL и которые не PRIVATE_TO_AUTHOR для других;
    -- видеть напоминания, которые относятся к нему (и, опционально, общие напоминания пространства).
  -- Не может:
    -- создавать заметки;
    -- редактировать или удалять любые заметки;
    -- создавать/редактировать/останавливать напоминания.
  -- Не управляет участниками и настройками пространства.

DO $$
DECLARE
  owner_id  BIGINT;
  admin_id  BIGINT;
  editor_id BIGINT;
  viewer_id BIGINT;
BEGIN
  SELECT id INTO owner_id  FROM spaces.space_role WHERE code = 'OWNER';
  SELECT id INTO admin_id  FROM spaces.space_role WHERE code = 'ADMIN';
  SELECT id INTO editor_id FROM spaces.space_role WHERE code = 'EDITOR';
  SELECT id INTO viewer_id FROM spaces.space_role WHERE code = 'VIEWER';

  -- Очистить существующие права (опционально, осторожно на проде)
--   DELETE FROM space_role_permission
--   WHERE role_id IN (owner_id, admin_id, editor_id, viewer_id);

  -- OWNER: может всё
  INSERT INTO spaces.space_role_permission (role_id, permission) VALUES
    (owner_id, 'SPACE_MANAGE_SETTINGS'),
    (owner_id, 'SPACE_MANAGE_INVITES'),
    (owner_id, 'SPACE_MANAGE_ROLES'),
    (owner_id, 'MEMBER_KICK'),

    (owner_id, 'NOTE_VIEW_ALL'),
    (owner_id, 'NOTE_CREATE'),
    (owner_id, 'NOTE_EDIT_ANY'),
    (owner_id, 'NOTE_DELETE_ANY'),

    (owner_id, 'REMINDER_CREATE'),
    (owner_id, 'REMINDER_VIEW_ALL'),
    (owner_id, 'REMINDER_EDIT_ANY'),
    (owner_id, 'REMINDER_DELETE_ANY')
    
    ON CONFLICT (role_id, permission) DO NOTHING;

  -- ADMIN: почти всё, кроме, например, управления ролями/владельца
  INSERT INTO spaces.space_role_permission (role_id, permission) VALUES
    (admin_id, 'SPACE_MANAGE_SETTINGS'),
    (admin_id, 'SPACE_MANAGE_INVITES'),
    (admin_id, 'MEMBER_KICK'),

    (admin_id, 'NOTE_VIEW_ALL'),
    (admin_id, 'NOTE_CREATE'),
    (admin_id, 'NOTE_EDIT_ANY'),
    (admin_id, 'NOTE_DELETE_ANY'),

    (admin_id, 'REMINDER_CREATE'),
    (admin_id, 'REMINDER_VIEW_ALL'),
    (admin_id, 'REMINDER_EDIT_ANY'),
    (admin_id, 'REMINDER_DELETE_ANY')

    ON CONFLICT (role_id, permission) DO NOTHING;

  -- EDITOR: видит всё, создаёт и редактирует, но не админит участников
  INSERT INTO spaces.space_role_permission (role_id, permission) VALUES
    (editor_id, 'NOTE_VIEW_ALL'),
    (editor_id, 'NOTE_CREATE'),
    (editor_id, 'NOTE_EDIT_OWN'),
    (editor_id, 'NOTE_EDIT_ANY'),     -- можно убрать, если хочешь только свои
    (editor_id, 'NOTE_DELETE_OWN'),

    (editor_id, 'REMINDER_CREATE'),
    (editor_id, 'REMINDER_VIEW_ALL'),
    (editor_id, 'REMINDER_EDIT_OWN')

    ON CONFLICT (role_id, permission) DO NOTHING;

  -- VIEWER: только чтение
  INSERT INTO spaces.space_role_permission (role_id, permission) VALUES
    (viewer_id, 'NOTE_VIEW_ALL'),
    (viewer_id, 'REMINDER_VIEW_ALL')

    ON CONFLICT (role_id, permission) DO NOTHING;
END $$;

-- Права на пространство
  -- SPACE_MANAGE_SETTINGS
  -- Может менять настройки пространства: название, описание, аватар, дефолтную роль, возможные политики видимости.

  -- SPACE_MANAGE_INVITES
  -- Может приглашать новых участников в пространство и удалять/отклонять приглашения.

  -- SPACE_MANAGE_ROLES
  -- Может менять роли участников (повышать/понижать), кроме, например, владельца (это уже бизнес‑логика).

  -- MEMBER_KICK
  -- Может исключать участников из пространства (переводить в BLOCKED/удалять).

-- Права на заметки
  -- NOTE_VIEW_ALL
  -- Может видеть все заметки в пространстве (кроме тех, что скрыты ACL или PRIVATE_TO_AUTHOR у других).

  -- NOTE_CREATE
  -- Может создавать новые заметки в пространстве.

  -- NOTE_EDIT_OWN
  -- Может редактировать только свои заметки.

  -- NOTE_EDIT_ANY
  -- Может редактировать любые заметки в пространстве (если не запрещено ACL).

  -- NOTE_DELETE_OWN
  -- Может удалять только свои заметки.

  -- NOTE_DELETE_ANY
  -- Может удалять любые заметки в пространстве.

  -- (Опционально, если нужно отдельно управление ACL заметок)
  -- NOTE_MANAGE_ACL
  -- Может настраивать видимость конкретных заметок (создавать/менять записи в note_acl: скрывать от участников, давать точечные права).

-- Права на напоминания
  -- REMINDER_CREATE
  -- Может создавать новые напоминания в пространстве.

  -- REMINDER_VIEW_ALL
  -- Может видеть все напоминания пространства (их описание/метаданные), а не только адресованные ему.

  -- REMINDER_EDIT_OWN
  -- Может редактировать только те напоминания, которые создал сам.

  -- REMINDER_EDIT_ANY
  -- Может редактировать любые напоминания в пространстве.

  -- REMINDER_DELETE_OWN
  -- Может удалять (отменять) только свои напоминания.

  -- REMINDER_DELETE_ANY
  -- Может удалять любые напоминания в пространстве.

  -- REMINDER_STOP_OWN
  -- Может останавливать доставку напоминания для себя (если есть запись в reminder_recipient).

  -- REMINDER_STOP_ANY
  -- Может останавливать напоминания для любых участников (например, админ/owner гасит «спамное» напоминание всем).

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_type WHERE typname = 'member_status'
  ) THEN
    -- Участник пространства
    CREATE TYPE member_status AS ENUM ('ACTIVE', 'INVITED', 'BLOCKED');
  END IF;
END $$;

-- ACTIVE - активный участник пространства
-- INVITED - ожидание ответа на приглашение
-- BLOCKED - исключен из пространства

-- При удалении роли переводим участников в роль VIEWER (триггер ниже)
CREATE TABLE IF NOT EXISTS spaces.space_member (
    space_id    uuid NOT NULL REFERENCES spaces.spaces(id) ON DELETE CASCADE,
    user_id     uuid NOT NULL REFERENCES users.telegram(id) ON DELETE CASCADE,
    role_id     BIGINT NOT NULL REFERENCES spaces.space_role(id) ON DELETE NO ACTION,
    invited_by  uuid REFERENCES users.telegram(id) ON DELETE SET NULL,
    status      member_status NOT NULL DEFAULT 'INVITED',

    can_invite  BOOLEAN NOT NULL DEFAULT true,   -- NULL = по роли, TRUE = разрешаем даже если роль не может

    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),

    PRIMARY KEY (space_id, user_id)
);

-- При удалении роли переводим участников в роль VIEWER
CREATE OR REPLACE FUNCTION spaces.on_space_role_delete_set_viewer()
RETURNS TRIGGER AS $$
DECLARE
  viewer_role_id BIGINT;
BEGIN
  IF OLD.code = 'VIEWER' THEN
    RAISE EXCEPTION 'Cannot delete VIEWER role: it is used as fallback when other roles are removed';
  END IF;
  SELECT id INTO viewer_role_id FROM spaces.space_role WHERE code = 'VIEWER' LIMIT 1;
  IF viewer_role_id IS NULL THEN
    RAISE EXCEPTION 'VIEWER role not found';
  END IF;
  UPDATE spaces.space_member SET role_id = viewer_role_id WHERE role_id = OLD.id;
  RETURN OLD;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS tr_space_role_delete_set_viewer ON spaces.space_role;
CREATE TRIGGER tr_space_role_delete_set_viewer
  BEFORE DELETE ON spaces.space_role
  FOR EACH ROW EXECUTE FUNCTION spaces.on_space_role_delete_set_viewer();
