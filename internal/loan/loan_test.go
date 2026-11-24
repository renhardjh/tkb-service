package loan

import (
	"testing"
	"time"

	"github.com/renhardjh/tkb-service/internal/dates"
)

func d(s string) time.Time { return dates.MustParse(s) }

func sampleLoan() Loan {
	return Loan{
		ID:           1,
		Principal:    3_000_000,
		RateBPS:      1800,
		TenorMonths:  3,
		DisbursedAt:  d("2025-01-15"),
		Installments: Schedule(3_000_000, 1800, 3, d("2025-01-15")),
	}
}

func TestSchedule(t *testing.T) {
	items := Schedule(1_000_000, 1200, 3, d("2025-01-31"))
	if len(items) != 3 {
		t.Fatalf("got %d installments", len(items))
	}
	wantDue := []string{"2025-02-28", "2025-03-31", "2025-04-30"}
	var total int64
	for i, inst := range items {
		if got := dates.Format(inst.DueDate); got != wantDue[i] {
			t.Errorf("installment %d due %s, want %s", inst.Seq, got, wantDue[i])
		}
		if inst.Interest != 10_000 {
			t.Errorf("installment %d interest %d, want 10000", inst.Seq, inst.Interest)
		}
		total += inst.Principal
	}
	if total != 1_000_000 {
		t.Errorf("principal sums to %d", total)
	}
	if items[2].Principal != 333_334 {
		t.Errorf("last installment should absorb remainder, got %d", items[2].Principal)
	}
}

func TestOutstandingAsOf(t *testing.T) {
	l := sampleLoan()
	l.Installments[0].Repayments = []Repayment{{Principal: 1_000_000, Interest: 45_000, PaidAt: d("2025-02-14")}}
	l.Installments[1].Repayments = []Repayment{{Principal: 400_000, PaidAt: d("2025-03-20")}}

	cases := []struct {
		asOf string
		want int64
	}{
		{"2025-01-14", 0},
		{"2025-01-15", 3_000_000},
		{"2025-02-14", 2_000_000},
		{"2025-03-19", 2_000_000},
		{"2025-03-20", 1_600_000},
	}
	for _, c := range cases {
		if got := l.OutstandingAsOf(d(c.asOf)); got != c.want {
			t.Errorf("OutstandingAsOf(%s) = %d, want %d", c.asOf, got, c.want)
		}
	}
}

func TestDaysPastDue(t *testing.T) {
	l := sampleLoan()
	l.Installments[0].Repayments = []Repayment{{Principal: 1_000_000, Interest: 45_000, PaidAt: d("2025-02-20")}}
	l.Installments[1].Repayments = []Repayment{{Principal: 1_000_000, Interest: 10_000, PaidAt: d("2025-03-15")}}

	cases := []struct {
		asOf string
		want int
	}{
		{"2025-02-15", 0},
		{"2025-02-16", 1},
		{"2025-02-19", 4},
		{"2025-02-20", 0},
		{"2025-03-20", 5},
		{"2025-04-15", 31},
		{"2025-06-14", 91},
	}
	for _, c := range cases {
		if got := l.DaysPastDue(d(c.asOf)); got != c.want {
			t.Errorf("DaysPastDue(%s) = %d, want %d", c.asOf, got, c.want)
		}
	}
}

func TestClosedLoanHasNoExposure(t *testing.T) {
	l := sampleLoan()
	closed := d("2025-02-01")
	l.ClosedAt = &closed
	l.Installments[0].Repayments = []Repayment{{Principal: 3_000_000, Interest: 45_000, PaidAt: closed}}

	if got := l.OutstandingAsOf(d("2025-01-31")); got != 3_000_000 {
		t.Errorf("before close outstanding = %d", got)
	}
	if got := l.OutstandingAsOf(d("2025-06-01")); got != 0 {
		t.Errorf("after close outstanding = %d", got)
	}
	if got := l.DaysPastDue(d("2025-06-01")); got != 0 {
		t.Errorf("after close dpd = %d", got)
	}
}
