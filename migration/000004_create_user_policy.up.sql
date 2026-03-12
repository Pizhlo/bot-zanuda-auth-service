CREATE SCHEMA IF NOT EXISTS policies;

-- Whitelist для приглашений (если нужно ONLY_WHITELIST)
CREATE TABLE IF NOT EXISTS policies.user_invite_whitelist (
    user_id            uuid NOT NULL REFERENCES users.telegram(id) ON DELETE CASCADE,
    allowed_inviter_id uuid NOT NULL REFERENCES users.telegram(id) ON DELETE CASCADE,

    PRIMARY KEY (user_id, allowed_inviter_id)
);
