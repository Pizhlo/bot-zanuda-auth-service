-- Удаляем таблицу глобальных админов
DROP TABLE IF EXISTS users.user_admin;

-- Удаляем индекс по service_clients
DROP INDEX IF EXISTS auth.idx_service_clients_client_id;

-- Удаляем таблицу машинных аккаунтов
DROP TABLE IF EXISTS auth.service_clients;

DROP SCHEMA IF EXISTS auth;