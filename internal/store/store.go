package store

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"github.com/renhardjh/tkb-service/internal/dates"
	"github.com/renhardjh/tkb-service/internal/loan"
	"github.com/renhardjh/tkb-service/internal/tkb"
)

//go:embed schema.sql
var schema string

var ErrNotFound = errors.New("not found")

type Store struct {
	db *sql.DB
}

type Snapshot struct {
	Period           string      `json:"period"`
	AsOf             string      `json:"as_of"`
	RunningLoans     int         `json:"running_loans"`
	TotalOutstanding int64       `json:"total_outstanding"`
	TKB0             tkb.Percent `json:"tkb0"`
	TKB30            tkb.Percent `json:"tkb30"`
	TKB60            tkb.Percent `json:"tkb60"`
	TKB90            tkb.Percent `json:"tkb90"`
	CreatedAt        time.Time   `json:"created_at"`
}

func Open(ctx context.Context, path string) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *Store) Reset(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM repayments; DELETE FROM installments; DELETE FROM loans; DELETE FROM tkb_snapshots;`)
	return err
}

func (s *Store) LoansAsOf(ctx context.Context, asOf time.Time) ([]loan.Loan, error) {
	day := dates.Format(asOf)
	rows, err := s.db.QueryContext(ctx, `
		SELECT l.id, l.borrower_id, l.principal, l.rate_bps, l.tenor_months, l.disbursed_at, l.closed_at,
		       i.id, i.seq, i.due_date, i.principal, i.interest
		FROM loans l
		JOIN installments i ON i.loan_id = l.id
		WHERE l.disbursed_at <= ?1 AND (l.closed_at IS NULL OR l.closed_at > ?1)
		ORDER BY l.id, i.seq`, day)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var loans []loan.Loan
	index := map[int64][2]int{}
	for rows.Next() {
		var (
			l                  loan.Loan
			inst               loan.Installment
			disbursed, dueDate string
			closed             sql.NullString
		)
		if err := rows.Scan(&l.ID, &l.BorrowerID, &l.Principal, &l.RateBPS, &l.TenorMonths, &disbursed, &closed,
			&inst.ID, &inst.Seq, &dueDate, &inst.Principal, &inst.Interest); err != nil {
			return nil, err
		}
		if inst.DueDate, err = dates.Parse(dueDate); err != nil {
			return nil, err
		}
		if n := len(loans); n == 0 || loans[n-1].ID != l.ID {
			if l.DisbursedAt, err = dates.Parse(disbursed); err != nil {
				return nil, err
			}
			if closed.Valid {
				t, err := dates.Parse(closed.String)
				if err != nil {
					return nil, err
				}
				l.ClosedAt = &t
			}
			loans = append(loans, l)
		}
		last := &loans[len(loans)-1]
		last.Installments = append(last.Installments, inst)
		index[inst.ID] = [2]int{len(loans) - 1, len(last.Installments) - 1}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	payments, err := s.db.QueryContext(ctx, `
		SELECT r.installment_id, r.principal, r.interest, r.paid_at
		FROM repayments r
		JOIN installments i ON i.id = r.installment_id
		JOIN loans l ON l.id = i.loan_id
		WHERE r.paid_at <= ?1 AND l.disbursed_at <= ?1 AND (l.closed_at IS NULL OR l.closed_at > ?1)
		ORDER BY r.paid_at`, day)
	if err != nil {
		return nil, err
	}
	defer payments.Close()

	for payments.Next() {
		var (
			installmentID int64
			r             loan.Repayment
			paidAt        string
		)
		if err := payments.Scan(&installmentID, &r.Principal, &r.Interest, &paidAt); err != nil {
			return nil, err
		}
		if r.PaidAt, err = dates.Parse(paidAt); err != nil {
			return nil, err
		}
		pos, ok := index[installmentID]
		if !ok {
			continue
		}
		inst := &loans[pos[0]].Installments[pos[1]]
		inst.Repayments = append(inst.Repayments, r)
	}
	return loans, payments.Err()
}

func (s *Store) SaveSnapshot(ctx context.Context, period string, r tkb.Report) (Snapshot, error) {
	snap := Snapshot{
		Period:           period,
		AsOf:             r.AsOf,
		RunningLoans:     r.RunningLoans,
		TotalOutstanding: r.TotalOutstanding,
		CreatedAt:        time.Now().UTC().Truncate(time.Second),
	}
	for _, target := range []struct {
		days int
		dst  *tkb.Percent
	}{{0, &snap.TKB0}, {30, &snap.TKB30}, {60, &snap.TKB60}, {90, &snap.TKB90}} {
		rate, ok := r.Rate(target.days)
		if !ok {
			return Snapshot{}, fmt.Errorf("report has no TKB%d rate", target.days)
		}
		*target.dst = rate.TKB
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO tkb_snapshots (period, as_of, running_loans, total_outstanding, tkb0, tkb30, tkb60, tkb90, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (period) DO UPDATE SET
			as_of = excluded.as_of,
			running_loans = excluded.running_loans,
			total_outstanding = excluded.total_outstanding,
			tkb0 = excluded.tkb0,
			tkb30 = excluded.tkb30,
			tkb60 = excluded.tkb60,
			tkb90 = excluded.tkb90,
			created_at = excluded.created_at`,
		snap.Period, snap.AsOf, snap.RunningLoans, snap.TotalOutstanding,
		int64(snap.TKB0), int64(snap.TKB30), int64(snap.TKB60), int64(snap.TKB90),
		snap.CreatedAt.Format(time.RFC3339))
	return snap, err
}

func (s *Store) Snapshots(ctx context.Context, from, to string) ([]Snapshot, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT period, as_of, running_loans, total_outstanding, tkb0, tkb30, tkb60, tkb90, created_at
		FROM tkb_snapshots
		WHERE period BETWEEN ? AND ?
		ORDER BY period`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	snaps := []Snapshot{}
	for rows.Next() {
		var (
			s         Snapshot
			createdAt string
		)
		if err := rows.Scan(&s.Period, &s.AsOf, &s.RunningLoans, &s.TotalOutstanding,
			&s.TKB0, &s.TKB30, &s.TKB60, &s.TKB90, &createdAt); err != nil {
			return nil, err
		}
		if s.CreatedAt, err = time.Parse(time.RFC3339, createdAt); err != nil {
			return nil, err
		}
		snaps = append(snaps, s)
	}
	return snaps, rows.Err()
}

func (s *Store) Snapshot(ctx context.Context, period string) (Snapshot, error) {
	snaps, err := s.Snapshots(ctx, period, period)
	if err != nil {
		return Snapshot{}, err
	}
	if len(snaps) == 0 {
		return Snapshot{}, ErrNotFound
	}
	return snaps[0], nil
}

func (s *Store) InsertLoans(ctx context.Context, loans []loan.Loan) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	insLoan, err := tx.PrepareContext(ctx, `INSERT INTO loans (borrower_id, principal, rate_bps, tenor_months, disbursed_at, closed_at) VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer insLoan.Close()
	insInst, err := tx.PrepareContext(ctx, `INSERT INTO installments (loan_id, seq, due_date, principal, interest) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer insInst.Close()
	insPay, err := tx.PrepareContext(ctx, `INSERT INTO repayments (installment_id, principal, interest, paid_at) VALUES (?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer insPay.Close()

	for _, l := range loans {
		var closed any
		if l.ClosedAt != nil {
			closed = dates.Format(*l.ClosedAt)
		}
		res, err := insLoan.ExecContext(ctx, l.BorrowerID, l.Principal, l.RateBPS, l.TenorMonths, dates.Format(l.DisbursedAt), closed)
		if err != nil {
			return err
		}
		loanID, err := res.LastInsertId()
		if err != nil {
			return err
		}
		for _, inst := range l.Installments {
			res, err := insInst.ExecContext(ctx, loanID, inst.Seq, dates.Format(inst.DueDate), inst.Principal, inst.Interest)
			if err != nil {
				return err
			}
			instID, err := res.LastInsertId()
			if err != nil {
				return err
			}
			for _, r := range inst.Repayments {
				if _, err := insPay.ExecContext(ctx, instID, r.Principal, r.Interest, dates.Format(r.PaidAt)); err != nil {
					return err
				}
			}
		}
	}
	return tx.Commit()
}
