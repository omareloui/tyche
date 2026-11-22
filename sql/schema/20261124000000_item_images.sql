-- +goose Up
-- +goose StatementBegin
-- An item's picture (one at most), apart from items so listing them never
-- reads the blobs: a small WebP (receiptimg.Thumbnail).
CREATE TABLE item_images (
    item_id VARCHAR NOT NULL PRIMARY KEY REFERENCES items (id) ON DELETE CASCADE,
    data BLOB NOT NULL,
    content_type VARCHAR NOT NULL,
    -- sha256 of data: its url's version and ETag.
    hash VARCHAR NOT NULL,
    width INTEGER NOT NULL,
    height INTEGER NOT NULL,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE item_images;
-- +goose StatementEnd
