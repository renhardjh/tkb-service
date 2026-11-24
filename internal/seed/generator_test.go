package seed

import (
	"reflect"
	"testing"

	"github.com/renhardjh/tkb-service/internal/dates"
)

func config() Config {
	return Config{
		Loans: 500,
		From:  dates.MustParse("2024-01-01"),
		To:    dates.MustParse("2025-06-30"),
		Now:   dates.MustParse("2025-07-31"),
		Seed:  7,
	}
}

func TestGenerateIsDeterministic(t *testing.T) {
	a, b := Generate(config()), Generate(config())
	if !reflect.DeepEqual(a, b) {
		t.Fatal("same seed produced different loan books")
	}
}

func TestGeneratedLoansAreConsistent(t *testing.T) {
	cfg := config()
	for _, l := range Generate(cfg) {
		if l.DisbursedAt.Before(cfg.From) || l.DisbursedAt.After(cfg.To) {
			t.Fatalf("loan disbursed outside range: %s", dates.Format(l.DisbursedAt))
		}
		var scheduled, paid int64
		for _, inst := range l.Installments {
			scheduled += inst.Principal
			for _, r := range inst.Repayments {
				if r.PaidAt.After(cfg.Now) {
					t.Fatalf("repayment after now: %s", dates.Format(r.PaidAt))
				}
				paid += r.Principal
			}
		}
		if scheduled != l.Principal {
			t.Fatalf("schedule principal %d != loan principal %d", scheduled, l.Principal)
		}
		if paid > l.Principal {
			t.Fatalf("overpaid loan: %d > %d", paid, l.Principal)
		}
		if l.ClosedAt != nil && paid != l.Principal {
			t.Fatalf("closed loan still has %d unpaid", l.Principal-paid)
		}
	}
}
