CREATE TABLE IF NOT EXISTS loans (
    id            INTEGER PRIMARY KEY,
    borrower_id   INTEGER NOT NULL,
    principal     INTEGER NOT NULL CHECK (principal > 0),
    rate_bps      INTEGER NOT NULL,
    tenor_months  INTEGER NOT NULL CHECK (tenor_months > 0),
    disbursed_at  TEXT    NOT NULL,
    closed_at     TEXT
);

CREATE INDEX IF NOT EXISTS idx_loans_disbursed_at ON loans (disbursed_at);

CREATE TABLE IF NOT EXISTS installments (
    id         INTEGER PRIMARY KEY,
    loan_id    INTEGER NOT NULL REFERENCES loans (id) ON DELETE CASCADE,
    seq        INTEGER NOT NULL,
    due_date   TEXT    NOT NULL,
    principal  INTEGER NOT NULL,
    interest   INTEGER NOT NULL,
    UNIQUE (loan_id, seq)
);

CREATE TABLE IF NOT EXISTS repayments (
    id              INTEGER PRIMARY KEY,
    installment_id  INTEGER NOT NULL REFERENCES installments (id) ON DELETE CASCADE,
    principal       INTEGER NOT NULL,
    interest        INTEGER NOT NULL,
    paid_at         TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_repayments_installment ON repayments (installment_id, paid_at);

CREATE TABLE IF NOT EXISTS tkb_snapshots (
    period             TEXT PRIMARY KEY,
    as_of              TEXT    NOT NULL,
    running_loans      INTEGER NOT NULL,
    total_outstanding  INTEGER NOT NULL,
    tkb0               INTEGER NOT NULL,
    tkb30              INTEGER NOT NULL,
    tkb60              INTEGER NOT NULL,
    tkb90              INTEGER NOT NULL,
    created_at         TEXT    NOT NULL
);
