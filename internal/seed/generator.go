package seed

import (
	"math/rand"
	"time"

	"github.com/renhardjh/tkb-service/internal/dates"
	"github.com/renhardjh/tkb-service/internal/loan"
)

type Behavior int

const (
	OnTime Behavior = iota
	Late
	EarlyPayoff
	Default
)

type Config struct {
	Loans int
	From  time.Time
	To    time.Time
	Now   time.Time
	Seed  int64
}

type profile struct {
	behavior Behavior
	weight   int
}

var profiles = []profile{
	{OnTime, 75},
	{Late, 13},
	{EarlyPayoff, 8},
	{Default, 4},
}

var tenors = []int{3, 6, 6, 12, 12, 12}

func Generate(cfg Config) []loan.Loan {
	rng := rand.New(rand.NewSource(cfg.Seed))
	span := dates.DaysBetween(cfg.From, cfg.To) + 1
	loans := make([]loan.Loan, 0, cfg.Loans)

	for n := 0; n < cfg.Loans; n++ {
		disbursed := cfg.From.AddDate(0, 0, rng.Intn(span))
		principal := int64(50+rng.Intn(1951)) * 100_000
		tenor := tenors[rng.Intn(len(tenors))]
		rate := int64(1400 + rng.Intn(13)*100)

		l := loan.Loan{
			BorrowerID:   int64(10_000 + rng.Intn(cfg.Loans*2)),
			Principal:    principal,
			RateBPS:      rate,
			TenorMonths:  tenor,
			DisbursedAt:  disbursed,
			Installments: loan.Schedule(principal, rate, tenor, disbursed),
		}
		applyRepayments(rng, &l, pick(rng), cfg.Now)
		loans = append(loans, l)
	}
	return loans
}

func pick(rng *rand.Rand) Behavior {
	total := 0
	for _, p := range profiles {
		total += p.weight
	}
	roll := rng.Intn(total)
	for _, p := range profiles {
		if roll < p.weight {
			return p.behavior
		}
		roll -= p.weight
	}
	return OnTime
}

func applyRepayments(rng *rand.Rand, l *loan.Loan, b Behavior, now time.Time) {
	tenor := len(l.Installments)
	stopAt := tenor
	payoffAt := -1

	switch b {
	case Default:
		stopAt = rng.Intn(tenor)
	case EarlyPayoff:
		payoffAt = rng.Intn(tenor)
	}

	var paidPrincipal int64
	for i := range l.Installments {
		inst := &l.Installments[i]

		if i == payoffAt {
			payDate := inst.DueDate.AddDate(0, 0, -rng.Intn(20))
			if payDate.Before(l.DisbursedAt) {
				payDate = l.DisbursedAt
			}
			if payDate.After(now) {
				return
			}
			inst.Repayments = append(inst.Repayments, loan.Repayment{
				Principal: l.Principal - paidPrincipal,
				Interest:  inst.Interest,
				PaidAt:    payDate,
			})
			l.ClosedAt = &payDate
			return
		}

		if i >= stopAt {
			if i == stopAt && rng.Intn(3) == 0 {
				payDate := inst.DueDate.AddDate(0, 0, rng.Intn(10))
				if !payDate.After(now) {
					inst.Repayments = append(inst.Repayments, loan.Repayment{
						Principal: inst.Principal / 2,
						PaidAt:    payDate,
					})
				}
			}
			return
		}

		payDate := inst.DueDate
		switch b {
		case Late:
			payDate = payDate.AddDate(0, 0, 1+rng.Intn(45))
		default:
			payDate = payDate.AddDate(0, 0, -rng.Intn(4))
		}
		if payDate.After(now) {
			return
		}
		inst.Repayments = append(inst.Repayments, loan.Repayment{
			Principal: inst.Principal,
			Interest:  inst.Interest,
			PaidAt:    payDate,
		})
		paidPrincipal += inst.Principal
	}

	var closed time.Time
	for _, inst := range l.Installments {
		for _, r := range inst.Repayments {
			if r.PaidAt.After(closed) {
				closed = r.PaidAt
			}
		}
	}
	l.ClosedAt = &closed
}
