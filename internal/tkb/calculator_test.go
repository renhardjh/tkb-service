package tkb

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/renhardjh/tkb-service/internal/dates"
	"github.com/renhardjh/tkb-service/internal/loan"
)

func d(s string) time.Time { return dates.MustParse(s) }

func overdueLoan(id, principal int64, dueDate string) loan.Loan {
	due := d(dueDate)
	return loan.Loan{
		ID:          id,
		Principal:   principal,
		TenorMonths: 1,
		DisbursedAt: due.AddDate(0, -1, 0),
		Installments: []loan.Installment{
			{Seq: 1, DueDate: due, Principal: principal, Interest: 0},
		},
	}
}

func TestCalculateUsesStrictlyGreaterThanThreshold(t *testing.T) {
	asOf := d("2025-07-31")
	loans := []loan.Loan{
		overdueLoan(1, 600_000_000, "2025-08-15"),
		overdueLoan(2, 100_000_000, dates.Format(asOf.AddDate(0, 0, -90))),
		overdueLoan(3, 200_000_000, dates.Format(asOf.AddDate(0, 0, -91))),
		overdueLoan(4, 100_000_000, dates.Format(asOf.AddDate(0, 0, -30))),
	}

	r := Calculate(loans, asOf)

	if r.RunningLoans != 4 || r.TotalOutstanding != 1_000_000_000 {
		t.Fatalf("running=%d outstanding=%d", r.RunningLoans, r.TotalOutstanding)
	}
	want := map[int]string{0: "60.00", 30: "70.00", 60: "70.00", 90: "80.00"}
	for days, tkbWant := range want {
		rate, _ := r.Rate(days)
		if rate.TKB.String() != tkbWant {
			t.Errorf("TKB%d = %s, want %s", days, rate.TKB, tkbWant)
		}
	}
	rate90, _ := r.Rate(90)
	if rate90.DelinquentLoans != 1 || rate90.DelinquentOutstanding != 200_000_000 {
		t.Errorf("TKB90 delinquent = %d loans / %d", rate90.DelinquentLoans, rate90.DelinquentOutstanding)
	}
}

func TestCalculateBuckets(t *testing.T) {
	asOf := d("2025-07-31")
	loans := []loan.Loan{
		overdueLoan(1, 100, "2025-08-10"),
		overdueLoan(2, 200, "2025-07-30"),
		overdueLoan(3, 300, dates.Format(asOf.AddDate(0, 0, -45))),
		overdueLoan(4, 400, dates.Format(asOf.AddDate(0, 0, -61))),
		overdueLoan(5, 500, dates.Format(asOf.AddDate(0, 0, -200))),
	}
	r := Calculate(loans, asOf)

	want := []struct {
		label string
		out   int64
	}{
		{"current", 100},
		{"dpd_1_30", 200},
		{"dpd_31_60", 300},
		{"dpd_61_90", 400},
		{"dpd_over_90", 500},
	}
	if len(r.Buckets) != len(want) {
		t.Fatalf("got %d buckets", len(r.Buckets))
	}
	for i, w := range want {
		b := r.Buckets[i]
		if b.Label != w.label || b.Outstanding != w.out || b.Loans != 1 {
			t.Errorf("bucket %d = %+v, want %s/%d", i, b, w.label, w.out)
		}
	}
}

func TestCalculateIgnoresRepaidAndFutureLoans(t *testing.T) {
	asOf := d("2025-07-31")
	repaid := overdueLoan(1, 500, "2025-07-01")
	repaid.Installments[0].Repayments = []loan.Repayment{{Principal: 500, PaidAt: d("2025-07-01")}}
	future := overdueLoan(2, 900, "2025-09-30")
	running := overdueLoan(3, 1_000, "2025-08-30")

	r := Calculate([]loan.Loan{repaid, future, running}, asOf)
	if r.RunningLoans != 1 || r.TotalOutstanding != 1_000 {
		t.Fatalf("running=%d outstanding=%d", r.RunningLoans, r.TotalOutstanding)
	}
}

func TestEmptyBookReportsFullRate(t *testing.T) {
	r := Calculate(nil, d("2025-07-31"))
	rate, _ := r.Rate(90)
	if rate.TKB.String() != "100.00" || rate.NPL.String() != "0.00" {
		t.Fatalf("empty book TKB90 = %s NPL90 = %s", rate.TKB, rate.NPL)
	}
}

func TestRatioRoundsHalfUp(t *testing.T) {
	cases := []struct {
		part, total int64
		want        string
	}{
		{1, 3, "33.33"},
		{2, 3, "66.67"},
		{1, 8, "12.50"},
		{1, 80_000, "0.00"},
		{1, 20_000, "0.01"},
		{9_000_000_000_000, 9_000_000_000_000, "100.00"},
	}
	for _, c := range cases {
		if got := Ratio(c.part, c.total).String(); got != c.want {
			t.Errorf("Ratio(%d, %d) = %s, want %s", c.part, c.total, got, c.want)
		}
	}
}

func TestPercentJSON(t *testing.T) {
	b, err := json.Marshal(map[string]Percent{"tkb90": 9712})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"tkb90":97.12}` {
		t.Fatalf("got %s", b)
	}
}
