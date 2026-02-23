INSERT INTO spaces.spaces (id, type, owner_id, default_participant_role) values (
    1, 'PERSONAL', 1, 'OWNER'
);

INSERT INTO spaces.space_member (space_id, user_id, role_id, status) VALUES (
    1, 1, (select id from spaces.space_role where code = 'OWNER'), 'ACTIVE'
);