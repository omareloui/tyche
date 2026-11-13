-- +goose Up
-- +goose StatementBegin
-- The rates of past days, fetched once when asked (fxrate.Job.RateOn), e.g.
-- to estimate what a receipt paid abroad came to on its day. Lowercase codes.
CREATE TABLE fx_rates_daily (
    day DATE NOT NULL,
    base VARCHAR NOT NULL,
    quote VARCHAR NOT NULL,
    rate FLOAT NOT NULL,
    PRIMARY KEY (day, base, quote)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE fx_rates_daily;
-- +goose StatementEnd
