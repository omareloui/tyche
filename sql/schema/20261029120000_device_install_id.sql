-- +goose Up
-- +goose StatementBegin
-- Stable per app install id, signing in again from the same install reuses
-- the device row.
ALTER TABLE devices ADD COLUMN install_id VARCHAR;
CREATE UNIQUE INDEX devices_install_id_idx ON devices (install_id);
-- Id of the device's current token, rotated on signing in again so older
-- tokens stop working. NULL for devices registered before this.
ALTER TABLE devices ADD COLUMN token_id VARCHAR;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE devices DROP COLUMN token_id;
DROP INDEX devices_install_id_idx;
ALTER TABLE devices DROP COLUMN install_id;
-- +goose StatementEnd
