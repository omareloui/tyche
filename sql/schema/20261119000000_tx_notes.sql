-- +goose Up
-- +goose StatementBegin
-- Notes (Markdown, stored as typed) of transactions and recurrent ones, the
-- latter copied to their occurrences.
ALTER TABLE transactions ADD COLUMN note VARCHAR NOT NULL DEFAULT '';
ALTER TABLE recurrent_txs ADD COLUMN note VARCHAR NOT NULL DEFAULT '';
DROP VIEW recurrent_txs_details;
CREATE VIEW recurrent_txs_details AS
SELECT
    rt.id AS recurrent_tx_id,
    rt.every AS recurrent_tx_every,
    rt.unit AS recurrent_tx_unit,
    rt.account_id AS recurrent_tx_account_id,
    rt.amount AS recurrent_tx_amount,
    rt.kind AS recurrent_tx_kind,
    rt.category_id AS recurrent_tx_category_id,
    rt.counterparty_id AS recurrent_tx_counterparty_id,
    rt.description AS recurrent_tx_description,
    rt.note AS recurrent_tx_note,
    rt.to_account_id AS recurrent_tx_to_account_id,
    rt.to_amount AS recurrent_tx_to_amount,
    rt.starts_at AS recurrent_tx_starts_at,
    rt.stops_at AS recurrent_tx_stops_at,
    rt.anchor AS recurrent_tx_anchor,
    rt.next_index AS recurrent_tx_next_index,
    rt.next_on AS recurrent_tx_next_on,
    rt.auto_fulfill AS recurrent_tx_auto_fulfill,
    rt.paused_at AS recurrent_tx_paused_at,
    rt.created_at AS recurrent_tx_created_at,

    -- Account details
    a.name AS account_name,
    a.kind_id AS account_kind_id,
    a.amount AS account_amount,
    a.currency_id AS account_currency_id,
    a.counterparty_id AS account_counterparty_id,
    a.card_number AS account_card_number,
    a.card_limit AS account_card_limit,
    a.name_on_card AS account_name_on_card,
    a.iban AS account_iban,
    a.note AS account_note,
    a.is_archived AS account_is_archived,
    a.exclude_from_balance AS account_exclude_from_balance,
    a.expiration_date AS account_expiration_date,
    a.created_at AS account_created_at,

    -- Account Currency
    c.string AS account_currency_string,
    c.symbol AS account_currency_symbol,
    c.in_grams AS account_currency_in_grams,

    -- Counterparty
    cp.name AS counterparty_name,

    -- To Account details
    ta.name AS to_account_name,
    ta.kind_id AS to_account_kind_id,
    ta.amount AS to_account_amount,
    ta.currency_id AS to_account_currency_id,
    ta.counterparty_id AS to_account_counterparty_id,
    ta.card_number AS to_account_card_number,
    ta.card_limit AS to_account_card_limit,
    ta.name_on_card AS to_account_name_on_card,
    ta.iban AS to_account_iban,
    ta.note AS to_account_note,
    ta.is_archived AS to_account_is_archived,
    ta.exclude_from_balance AS to_account_exclude_from_balance,
    ta.expiration_date AS to_account_expiration_date,
    ta.created_at AS to_account_created_at,

    -- To Account Currency
    tc.string AS to_account_currency_string,
    tc.symbol AS to_account_currency_symbol,
    tc.in_grams AS to_account_currency_in_grams,

    -- Category
    cat.name AS category_name,
    cat.budget_amount AS category_budget_amount,
    cat.budget_currency_id AS category_budget_currency_id,
    cat.kinds AS category_kinds,
    cat.created_at AS category_created_at,

    -- The gam'yaa it pays the installments of, if any
    g.account_id AS gamyaa_account_id

FROM recurrent_txs AS rt
LEFT JOIN counterparties AS cp ON rt.counterparty_id = cp.id
LEFT JOIN accounts AS a ON rt.account_id = a.id
LEFT JOIN currencies AS c ON a.currency_id = c.id
LEFT JOIN accounts AS ta ON rt.to_account_id = ta.id
LEFT JOIN currencies AS tc ON ta.currency_id = tc.id
LEFT JOIN categories AS cat ON rt.category_id = cat.id
LEFT JOIN gamyaas AS g ON g.recurrent_tx_id = rt.id;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP VIEW recurrent_txs_details;
ALTER TABLE recurrent_txs DROP COLUMN note;
ALTER TABLE transactions DROP COLUMN note;
CREATE VIEW recurrent_txs_details AS
SELECT
    rt.id AS recurrent_tx_id,
    rt.every AS recurrent_tx_every,
    rt.unit AS recurrent_tx_unit,
    rt.account_id AS recurrent_tx_account_id,
    rt.amount AS recurrent_tx_amount,
    rt.kind AS recurrent_tx_kind,
    rt.category_id AS recurrent_tx_category_id,
    rt.counterparty_id AS recurrent_tx_counterparty_id,
    rt.description AS recurrent_tx_description,
    rt.to_account_id AS recurrent_tx_to_account_id,
    rt.to_amount AS recurrent_tx_to_amount,
    rt.starts_at AS recurrent_tx_starts_at,
    rt.stops_at AS recurrent_tx_stops_at,
    rt.anchor AS recurrent_tx_anchor,
    rt.next_index AS recurrent_tx_next_index,
    rt.next_on AS recurrent_tx_next_on,
    rt.auto_fulfill AS recurrent_tx_auto_fulfill,
    rt.paused_at AS recurrent_tx_paused_at,
    rt.created_at AS recurrent_tx_created_at,

    -- Account details
    a.name AS account_name,
    a.kind_id AS account_kind_id,
    a.amount AS account_amount,
    a.currency_id AS account_currency_id,
    a.counterparty_id AS account_counterparty_id,
    a.card_number AS account_card_number,
    a.card_limit AS account_card_limit,
    a.name_on_card AS account_name_on_card,
    a.iban AS account_iban,
    a.note AS account_note,
    a.is_archived AS account_is_archived,
    a.exclude_from_balance AS account_exclude_from_balance,
    a.expiration_date AS account_expiration_date,
    a.created_at AS account_created_at,

    -- Account Currency
    c.string AS account_currency_string,
    c.symbol AS account_currency_symbol,
    c.in_grams AS account_currency_in_grams,

    -- Counterparty
    cp.name AS counterparty_name,

    -- To Account details
    ta.name AS to_account_name,
    ta.kind_id AS to_account_kind_id,
    ta.amount AS to_account_amount,
    ta.currency_id AS to_account_currency_id,
    ta.counterparty_id AS to_account_counterparty_id,
    ta.card_number AS to_account_card_number,
    ta.card_limit AS to_account_card_limit,
    ta.name_on_card AS to_account_name_on_card,
    ta.iban AS to_account_iban,
    ta.note AS to_account_note,
    ta.is_archived AS to_account_is_archived,
    ta.exclude_from_balance AS to_account_exclude_from_balance,
    ta.expiration_date AS to_account_expiration_date,
    ta.created_at AS to_account_created_at,

    -- To Account Currency
    tc.string AS to_account_currency_string,
    tc.symbol AS to_account_currency_symbol,
    tc.in_grams AS to_account_currency_in_grams,

    -- Category
    cat.name AS category_name,
    cat.budget_amount AS category_budget_amount,
    cat.budget_currency_id AS category_budget_currency_id,
    cat.kinds AS category_kinds,
    cat.created_at AS category_created_at,

    -- The gam'yaa it pays the installments of, if any
    g.account_id AS gamyaa_account_id

FROM recurrent_txs AS rt
LEFT JOIN counterparties AS cp ON rt.counterparty_id = cp.id
LEFT JOIN accounts AS a ON rt.account_id = a.id
LEFT JOIN currencies AS c ON a.currency_id = c.id
LEFT JOIN accounts AS ta ON rt.to_account_id = ta.id
LEFT JOIN currencies AS tc ON ta.currency_id = tc.id
LEFT JOIN categories AS cat ON rt.category_id = cat.id
LEFT JOIN gamyaas AS g ON g.recurrent_tx_id = rt.id;
-- +goose StatementEnd
