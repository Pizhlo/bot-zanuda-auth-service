-- enum note_visibility_type, note_acl_access
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_type WHERE typname = 'note_visibility_type') THEN
    DROP TYPE note_visibility_type CASCADE;
  END IF;

  IF EXISTS (SELECT 1 FROM pg_type WHERE typname = 'note_acl_access') THEN
    DROP TYPE note_acl_access CASCADE;
  END IF;
END $$;

DROP INDEX IF EXISTS idx_note_space;
DROP INDEX IF EXISTS idx_note_acl_note_user;

ALTER TABLE notes.note_acl DROP CONSTRAINT IF EXISTS note_acl_deny_no_flags;

DROP TABLE IF EXISTS notes.note_acl CASCADE;
DROP TABLE IF EXISTS notes.notes CASCADE;

DROP SCHEMA IF EXISTS notes CASCADE;