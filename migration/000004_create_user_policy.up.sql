CREATE SCHEMA IF NOT EXISTS policies;

-- Whitelist для приглашений (если нужно ONLY_WHITELIST)
CREATE TABLE IF NOT EXISTS policies.user_invite_whitelist (
    user_id           BIGINT NOT NULL REFERENCES users.users(id) ON DELETE CASCADE,
    allowed_inviter_id BIGINT NOT NULL REFERENCES users.users(id) ON DELETE CASCADE,

    PRIMARY KEY (user_id, allowed_inviter_id)
);
