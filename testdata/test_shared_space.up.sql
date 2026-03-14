-- новые пользователи - тестовые участники 
INSERT INTO users.telegram (id, tg_id, global_invite_policy) VALUES (
    'd1b3a949-8565-4268-8c72-6d27247cbaa5', 22222, 'ALLOW_ALL' -- ADMIN
);

INSERT INTO users.telegram (id, tg_id, global_invite_policy) VALUES (
    '33fd6e4c-26d3-45a6-93e2-3a0514cfac5a', 33333, 'ALLOW_ALL' -- EDITOR
);

INSERT INTO users.telegram (id, tg_id, global_invite_policy) VALUES (
    '0e9c136b-eee9-4d2b-bba3-fc32a9b5f2b4', 44444, 'ALLOW_ALL' -- VIEWER
);

-- совместное пространство
INSERT INTO spaces.spaces (id, type, owner_id, default_participant_role) values (
    '7cc54caa-1753-4839-aa0c-6f2a76a08e93', 'SHARED', '56279a7c-13a0-4464-98fe-8cee52bcd3b7', 'EDITOR'
);

-- новые участники
INSERT INTO spaces.space_member (space_id, user_id, role_id, invited_by, status) VALUES (
    '7cc54caa-1753-4839-aa0c-6f2a76a08e93', '56279a7c-13a0-4464-98fe-8cee52bcd3b7', (select id from spaces.space_role where code = 'OWNER'), '56279a7c-13a0-4464-98fe-8cee52bcd3b7', 'ACTIVE'
);

INSERT INTO spaces.space_member (space_id, user_id, role_id, invited_by, status) VALUES (
    '7cc54caa-1753-4839-aa0c-6f2a76a08e93', 'd1b3a949-8565-4268-8c72-6d27247cbaa5', (select id from spaces.space_role where code = 'ADMIN'), '56279a7c-13a0-4464-98fe-8cee52bcd3b7', 'ACTIVE'
);

INSERT INTO spaces.space_member (space_id, user_id, role_id, invited_by, status) VALUES (
    '7cc54caa-1753-4839-aa0c-6f2a76a08e93', '33fd6e4c-26d3-45a6-93e2-3a0514cfac5a', (select id from spaces.space_role where code = 'EDITOR'), '56279a7c-13a0-4464-98fe-8cee52bcd3b7', 'ACTIVE'
);

INSERT INTO spaces.space_member (space_id, user_id, role_id, invited_by, status) VALUES (
    '7cc54caa-1753-4839-aa0c-6f2a76a08e93', '0e9c136b-eee9-4d2b-bba3-fc32a9b5f2b4', (select id from spaces.space_role where code = 'VIEWER'), '56279a7c-13a0-4464-98fe-8cee52bcd3b7', 'ACTIVE'
);

-- вставляем несколько заметок в пространство
INSERT INTO notes.notes (id, space_id, author_id) VALUES (
    '1776f52a-8ce0-4bcd-9b75-0d3f2606883a', '7cc54caa-1753-4839-aa0c-6f2a76a08e93', '56279a7c-13a0-4464-98fe-8cee52bcd3b7' -- OWNER
);

INSERT INTO notes.notes (id, space_id, author_id) VALUES (
    'c50fddd1-fa97-4019-abcb-ad0a14d3cee7', '7cc54caa-1753-4839-aa0c-6f2a76a08e93', 'd1b3a949-8565-4268-8c72-6d27247cbaa5' -- ADMIN
);

INSERT INTO notes.notes (id, space_id, author_id) VALUES (
    '269a5ff9-ae0b-463c-860b-0d87b9fead16', '7cc54caa-1753-4839-aa0c-6f2a76a08e93', '33fd6e4c-26d3-45a6-93e2-3a0514cfac5a' -- EDITOR
);

INSERT INTO notes.notes (id, space_id, author_id) VALUES (
    '18bfb5da-3f5f-43b5-8b07-ef85887aec89', '7cc54caa-1753-4839-aa0c-6f2a76a08e93', '0e9c136b-eee9-4d2b-bba3-fc32a9b5f2b4' -- VIEWER
);