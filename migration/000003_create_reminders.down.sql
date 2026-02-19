DROP TABLE IF EXISTS reminders.reminder_recipient CASCADE;
DROP TABLE IF EXISTS reminders.reminders CASCADE;

DROP INDEX IF EXISTS idx_reminder_space;
DROP INDEX IF EXISTS idx_reminder_recipient_user;

DROP SCHEMA IF EXISTS reminders CASCADE;