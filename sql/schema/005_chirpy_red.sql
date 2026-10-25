-- +goose up
ALTER TABLE users ADD COLUMN is_chirpy_red BOOL DEFAULT false NOT NULL;

-- +goose down
ALTER TABLE users DROP COLUMN is_chirpy_red;