-- новые пользователи - тестовые участники 
INSERT INTO users.telegram (id, tg_id, global_invite_policy) VALUES (
    2, 22222, 'ALLOW_ALL' -- ADMIN
);

INSERT INTO users.telegram (id, tg_id, global_invite_policy) VALUES (
    3, 33333, 'ALLOW_ALL' -- EDITOR
);

INSERT INTO users.telegram (id, tg_id, global_invite_policy) VALUES (
    4, 44444, 'ALLOW_ALL' -- VIEWER
);

-- совместное пространство
INSERT INTO spaces.spaces (id, type, owner_id, default_participant_role) values (
    2, 'SHARED', 1, 'EDITOR'
);

-- новые участники
INSERT INTO spaces.space_member (space_id, user_id, role_id, status) VALUES (
    2, 1, (select id from spaces.space_role where code = 'OWNER'), 'ACTIVE'
);

INSERT INTO spaces.space_member (space_id, user_id, role_id, status) VALUES (
    2, 2, (select id from spaces.space_role where code = 'ADMIN'), 'ACTIVE'
);

INSERT INTO spaces.space_member (space_id, user_id, role_id, status) VALUES (
    2, 3, (select id from spaces.space_role where code = 'EDITOR'), 'ACTIVE'
);

INSERT INTO spaces.space_member (space_id, user_id, role_id, status) VALUES (
    2, 4, (select id from spaces.space_role where code = 'VIEWER'), 'ACTIVE'
);

-- вставляем несколько заметок в пространство
INSERT INTO notes.notes (space_id, author_id) VALUES (
    2, 1 -- OWNER
);

INSERT INTO notes.notes (space_id, author_id) VALUES (
    2, 2 -- ADMIN
);

INSERT INTO notes.notes (space_id, author_id) VALUES (
    2, 3 -- EDITOR
);