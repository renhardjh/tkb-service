package loan

import (
	"time"

	"github.com/renhardjh/tkb-service/internal/dates"
)

type Repayment struct {
	Principal int64
	Interest  int64
	PaidAt    time.Time
}

type Installment struct {
	ID         int64
	Seq        int
	DueDate    time.Time
	Principal  int64
	Interest   int64
	Repayments []Repayment
}

type Loan struct {
	ID           int64
	BorrowerID   int64
	Principal    int64
	RateBPS      int64
	TenorMonths  int
	DisbursedAt  time.Time
	ClosedAt     *time.Time
	Installments []Installment
}

func (i Installment) PaidAsOf(asOf time.Time) (principal, interest int64) {
	for _, r := range i.Repayments {
		if r.PaidAt.After(asOf) {
			continue
		}
		principal += r.Principal
		interest += r.Interest
	}
	return principal, interest
}

func (i Installment) SettledAsOf(asOf time.Time) bool {
	p, in := i.PaidAsOf(asOf)
	return p >= i.Principal && in >= i.Interest
}

func (l Loan) ClosedAsOf(asOf time.Time) bool {
	return l.ClosedAt != nil && !l.ClosedAt.After(asOf)
}

func (l Loan) OutstandingAsOf(asOf time.Time) int64 {
	if l.DisbursedAt.After(asOf) || l.ClosedAsOf(asOf) {
		return 0
	}
	var paid int64
	for _, inst := range l.Installments {
		p, _ := inst.PaidAsOf(asOf)
		paid += p
	}
	if paid >= l.Principal {
		return 0
	}
	return l.Principal - paid
}

func (l Loan) RunningAsOf(asOf time.Time) bool {
	return l.OutstandingAsOf(asOf) > 0
}

func (l Loan) DaysPastDue(asOf time.Time) int {
	if !l.RunningAsOf(asOf) {
		return 0
	}
	for _, inst := range l.Installments {
		if !inst.DueDate.Before(asOf) {
			return 0
		}
		if !inst.SettledAsOf(asOf) {
			return dates.DaysBetween(inst.DueDate, asOf)
		}
	}
	return 0
}

func Schedule(principal, rateBPS int64, tenor int, disbursedAt time.Time) []Installment {
	monthlyInterest := principal * rateBPS / 10000 / 12
	base := principal / int64(tenor)
	items := make([]Installment, tenor)
	var allocated int64
	for n := 1; n <= tenor; n++ {
		p := base
		if n == tenor {
			p = principal - allocated
		}
		allocated += p
		items[n-1] = Installment{
			Seq:       n,
			DueDate:   dates.AddMonths(disbursedAt, n),
			Principal: p,
			Interest:  monthlyInterest,
		}
	}
	return items
}
