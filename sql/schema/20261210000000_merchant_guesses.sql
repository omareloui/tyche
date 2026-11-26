-- +goose Up
-- +goose StatementBegin
-- ai_merchants turns matching the sms' unknown merchants with AI on.
ALTER TABLE user ADD COLUMN ai_merchants BOOLEAN NOT NULL DEFAULT FALSE;

-- merchant_guesses are the AI's decisions on an sms' merchant string no
-- counterparty matched: matched to a merchant (raw added as its alias) or a
-- merchant it created (created). A rejected one ("Wrong match") had its
-- alias removed and is never asked again.
CREATE TABLE merchant_guesses (
    id TEXT PRIMARY KEY,
    raw TEXT NOT NULL,
    counterparty_id TEXT NOT NULL REFERENCES counterparties (id) ON DELETE CASCADE,
    category_id TEXT REFERENCES categories (id) ON DELETE SET NULL,
    created BOOLEAN NOT NULL DEFAULT FALSE,
    rejected_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX merchant_guesses_raw ON merchant_guesses (raw COLLATE NOCASE);

-- merchant_guess_id is the guess that gave an sms transaction its merchant.
ALTER TABLE transactions ADD COLUMN merchant_guess_id TEXT REFERENCES merchant_guesses (id) ON DELETE SET NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE transactions DROP COLUMN merchant_guess_id;
DROP TABLE merchant_guesses;
ALTER TABLE user DROP COLUMN ai_merchants;
-- +goose StatementEnd
