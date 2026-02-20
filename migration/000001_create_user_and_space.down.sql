DROP TRIGGER IF EXISTS tr_space_role_delete_set_viewer ON spaces.space_role;

DROP TABLE IF EXISTS spaces.space_member CASCADE;
DROP TABLE IF EXISTS spaces.space_role_permission CASCADE;
DROP TABLE IF EXISTS spaces.space_role CASCADE;
DROP TABLE IF EXISTS spaces.spaces CASCADE;

-- enum member_status, space_type
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_type WHERE typname = 'member_status') THEN
    DROP TYPE member_status CASCADE;
  END IF;

  IF EXISTS (SELECT 1 FROM pg_type WHERE typname = 'space_type') THEN
    DROP TYPE space_type CASCADE;
  END IF;

  IF EXISTS (SELECT 1 FROM pg_type WHERE typname = 'space_role_code') THEN
    DROP TYPE space_role_code CASCADE;
  END IF;
END $$;

-- Пользователь
DROP TABLE IF EXISTS users.telegram CASCADE;

ALTER TABLE IF EXISTS public.schema_migrations
  DROP COLUMN created;

DROP SCHEMA IF EXISTS spaces CASCADE;
DROP SCHEMA IF EXISTS users CASCADE;