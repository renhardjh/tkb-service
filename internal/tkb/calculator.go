package tkb

import (
	"fmt"
	"time"

	"github.com/renhardjh/tkb-service/internal/dates"
	"github.com/renhardjh/tkb-service/internal/loan"
)

var Thresholds = []int{0, 30, 60, 90}

type Rate struct {
	Days                  int     `json:"days"`
	TKB                   Percent `json:"tkb"`
	NPL                   Percent `json:"npl"`
	DelinquentLoans       int     `json:"delinquent_loans"`
	DelinquentOutstanding int64   `json:"delinquent_outstanding"`
}

type Bucket struct {
	Label       string  `json:"label"`
	MinDPD      int     `json:"min_dpd"`
	MaxDPD      *int    `json:"max_dpd"`
	Loans       int     `json:"loans"`
	Outstanding int64   `json:"outstanding"`
	Share       Percent `json:"share"`
}

type Report struct {
	AsOf             string   `json:"as_of"`
	RunningLoans     int      `json:"running_loans"`
	TotalOutstanding int64    `json:"total_outstanding"`
	Rates            []Rate   `json:"rates"`
	Buckets          []Bucket `json:"buckets"`
}

func (r Report) Rate(days int) (Rate, bool) {
	for _, rate := range r.Rates {
		if rate.Days == days {
			return rate, true
		}
	}
	return Rate{}, false
}

func Calculate(loans []loan.Loan, asOf time.Time) Report {
	report := Report{AsOf: dates.Format(asOf), Buckets: newBuckets()}
	rates := make([]Rate, len(Thresholds))
	for i, days := range Thresholds {
		rates[i].Days = days
	}

	for _, l := range loans {
		outstanding := l.OutstandingAsOf(asOf)
		if outstanding == 0 {
			continue
		}
		dpd := l.DaysPastDue(asOf)
		report.RunningLoans++
		report.TotalOutstanding += outstanding

		for i := range rates {
			if dpd > rates[i].Days {
				rates[i].DelinquentLoans++
				rates[i].DelinquentOutstanding += outstanding
			}
		}
		b := &report.Buckets[bucketIndex(dpd)]
		b.Loans++
		b.Outstanding += outstanding
	}

	for i := range rates {
		rates[i].NPL = Ratio(rates[i].DelinquentOutstanding, report.TotalOutstanding)
		rates[i].TKB = hundredPercent - rates[i].NPL
	}
	for i := range report.Buckets {
		report.Buckets[i].Share = Ratio(report.Buckets[i].Outstanding, report.TotalOutstanding)
	}
	report.Rates = rates
	return report
}

func newBuckets() []Bucket {
	buckets := []Bucket{{Label: "current", MinDPD: 0, MaxDPD: intPtr(0)}}
	prev := 0
	for _, days := range Thresholds[1:] {
		buckets = append(buckets, Bucket{
			Label:  fmt.Sprintf("dpd_%d_%d", prev+1, days),
			MinDPD: prev + 1,
			MaxDPD: intPtr(days),
		})
		prev = days
	}
	return append(buckets, Bucket{Label: fmt.Sprintf("dpd_over_%d", prev), MinDPD: prev + 1})
}

func bucketIndex(dpd int) int {
	if dpd == 0 {
		return 0
	}
	for i, days := range Thresholds[1:] {
		if dpd <= days {
			return i + 1
		}
	}
	return len(Thresholds)
}

func intPtr(v int) *int {
	return &v
}
