-- +goose Up
-- +goose StatementBegin
-- The latest rate of each base → quote pair (lowercase codes), shared by all
-- users and refreshed by the fx rates job.
CREATE TABLE fx_rates (
    base VARCHAR NOT NULL,
    quote VARCHAR NOT NULL,
    rate REAL NOT NULL,
    as_of DATE NOT NULL,
    fetched_at DATETIME NOT NULL,
    PRIMARY KEY (base, quote)
) WITHOUT ROWID;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE fx_rates;
-- +goose StatementEnd
