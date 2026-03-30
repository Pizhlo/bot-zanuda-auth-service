CREATE SCHEMA IF NOT EXISTS auth;

-- машинные аккаунты для системного взаимодействия
CREATE TABLE auth.service_clients (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id           TEXT NOT NULL UNIQUE,          -- "bot", "web-server"
    client_name         TEXT NOT NULL,                 -- Человеческое имя
    scopes              TEXT[] NOT NULL DEFAULT '{}',  -- ['bot'], ['admin']
    is_active           BOOLEAN NOT NULL DEFAULT TRUE,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_service_clients_client_id
    ON auth.service_clients(client_id);

INSERT INTO auth.service_clients (client_id, client_name, scopes)
VALUES (
    'bot',
    'Telegram bot service account',
    ARRAY['bot']
);

-- Глобальные админы системы
CREATE TABLE users.user_admin (
    user_id        UUID PRIMARY KEY REFERENCES users.users(id) ON DELETE CASCADE,
    is_super_admin BOOLEAN NOT NULL DEFAULT TRUE,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by     UUID NULL REFERENCES users.users(id) ON DELETE SET NULL  -- кто сделал админом
);