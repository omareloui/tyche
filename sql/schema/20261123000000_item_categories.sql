-- +goose NO TRANSACTION
-- Items get categories of their own (e.g. Dairy, Snacks), apart from the
-- budget ones: items.category_id now points at item_categories. The budget
-- categories items had are copied as item categories (same id, name and
-- color). Changing a column's foreign key needs a table rebuild, with the
-- foreign keys off so dropping items doesn't clear transaction_items.item_id.

-- +goose Up
PRAGMA foreign_keys = OFF;

-- +goose StatementBegin
BEGIN;

CREATE TABLE item_categories (
    id VARCHAR UNIQUE NOT NULL PRIMARY KEY,
    name VARCHAR NOT NULL,
    -- One of models.CategoryColors.
    color VARCHAR NOT NULL DEFAULT 'gray',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX item_categories_name ON item_categories (name COLLATE NOCASE);

INSERT OR IGNORE INTO item_categories (id, name, color)
SELECT DISTINCT c.id, c.name, c.color
FROM categories AS c
JOIN items AS i ON i.category_id = c.id;

CREATE TABLE items_new (
    id VARCHAR UNIQUE NOT NULL PRIMARY KEY,
    name VARCHAR NOT NULL,
    name_key VARCHAR NOT NULL,
    aliases VARCHAR NOT NULL DEFAULT '[]',
    category_id VARCHAR REFERENCES item_categories (id) ON DELETE SET NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO items_new (id, name, name_key, aliases, category_id, created_at, updated_at)
SELECT i.id, i.name, i.name_key, i.aliases,
    (SELECT ic.id FROM item_categories AS ic WHERE ic.id = i.category_id),
    i.created_at, i.updated_at
FROM items AS i;
DROP TABLE items;
ALTER TABLE items_new RENAME TO items;
CREATE UNIQUE INDEX items_name_key ON items (name_key);

COMMIT;
-- +goose StatementEnd

PRAGMA foreign_keys = ON;

-- +goose Down
PRAGMA foreign_keys = OFF;

-- +goose StatementBegin
BEGIN;

CREATE TABLE items_old (
    id VARCHAR UNIQUE NOT NULL PRIMARY KEY,
    name VARCHAR NOT NULL,
    name_key VARCHAR NOT NULL,
    aliases VARCHAR NOT NULL DEFAULT '[]',
    category_id VARCHAR REFERENCES categories (id) ON DELETE SET NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO items_old (id, name, name_key, aliases, category_id, created_at, updated_at)
SELECT i.id, i.name, i.name_key, i.aliases,
    (SELECT c.id FROM categories AS c WHERE c.id = i.category_id),
    i.created_at, i.updated_at
FROM items AS i;
DROP TABLE items;
ALTER TABLE items_old RENAME TO items;
CREATE UNIQUE INDEX items_name_key ON items (name_key);
DROP TABLE item_categories;

COMMIT;
-- +goose StatementEnd

PRAGMA foreign_keys = ON;
