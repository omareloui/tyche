-- +goose Up
-- +goose StatementBegin
-- Price levels of the cost of living calculator (internal/costofliving):
-- purchasing power parities (local currency per international dollar) per
-- country (ISO 3166 alpha-3), series ('avg' = the World Bank's household
-- consumption PPP, else an ICP 2021 expenditure group's code) and year.
CREATE TABLE col_ppp (
    country VARCHAR NOT NULL,
    series VARCHAR NOT NULL,
    year INTEGER NOT NULL,
    value REAL NOT NULL,
    PRIMARY KEY (country, series, year)
) WITHOUT ROWID;

-- Consumer prices' yearly change (%) per country (IMF WEO, with its
-- estimates of the current years), rolling the PPPs to today.
CREATE TABLE col_inflation (
    country VARCHAR NOT NULL,
    year INTEGER NOT NULL,
    pct REAL NOT NULL,
    PRIMARY KEY (country, year)
) WITHOUT ROWID;

-- Gemini's city estimates (colguess), by the two cities and the month, so
-- asking again is free. No tenant: only the cities and the estimate.
CREATE TABLE col_city_estimates (
    key VARCHAR PRIMARY KEY,
    estimate TEXT NOT NULL,
    model VARCHAR NOT NULL,
    created_at DATETIME NOT NULL
) WITHOUT ROWID;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE col_city_estimates;
DROP TABLE col_inflation;
DROP TABLE col_ppp;
-- +goose StatementEnd
