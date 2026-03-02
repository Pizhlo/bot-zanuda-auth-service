DELETE FROM users.telegram where tg_id = 22222;
DELETE FROM users.telegram where tg_id = 33333;
DELETE FROM users.telegram where tg_id = 44444;

DELETE FROM spaces.spaces WHERE owner_id = 1 AND type = 'SHARED';