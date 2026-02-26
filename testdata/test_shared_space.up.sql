-- новый пользователь - тестовый участник 
INSERT INTO users.telegram (id, tg_id, global_invite_policy) VALUES (
    2, 22222, 'ALLOW_ALL'
);

INSERT INTO spaces.spaces (id, type, owner_id, default_participant_role) values (
    2, 'SHARED', 1, 'EDITOR'
);

INSERT INTO spaces.space_member (space_id, user_id, role_id, status) VALUES (
    2, 1, (select id from spaces.space_role where code = 'OWNER'), 'ACTIVE'
);

INSERT INTO spaces.space_member (space_id, user_id, role_id, status) VALUES (
    2, 2, (select id from spaces.space_role where code = 'EDITOR'), 'ACTIVE'
);

-- вставляем несколько заметок в пространство
INSERT INTO notes.notes (space_id, author_id) VALUES (
    2, 1
);

INSERT INTO notes.notes (space_id, author_id) VALUES (
    2, 1
);

INSERT INTO notes.notes (space_id, author_id) VALUES (
    2, 2
);