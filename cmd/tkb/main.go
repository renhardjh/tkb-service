package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/renhardjh/tkb-service/internal/dates"
	"github.com/renhardjh/tkb-service/internal/store"
	"github.com/renhardjh/tkb-service/internal/tkb"
)

func main() {
	dbPath := flag.String("db", "tkb.db", "sqlite database path")
	asOf := flag.String("as-of", dates.Format(dates.Today()), "calculation date (YYYY-MM-DD)")
	flag.Parse()

	if err := run(*dbPath, *asOf); err != nil {
		fmt.Fprintln(os.Stderr, "tkb:", err)
		os.Exit(1)
	}
}

func run(dbPath, asOfArg string) error {
	asOf, err := dates.Parse(asOfArg)
	if err != nil {
		return err
	}
	ctx := context.Background()
	db, err := store.Open(ctx, dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	loans, err := db.LoansAsOf(ctx, asOf)
	if err != nil {
		return err
	}
	r := tkb.Calculate(loans, asOf)

	fmt.Printf("As of %s: %d running loans, outstanding Rp%s\n\n", r.AsOf, r.RunningLoans, rupiah(r.TotalOutstanding))

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', tabwriter.AlignRight)
	fmt.Fprintln(tw, "METRIC\tTKB %\tNPL %\tLOANS\tOUTSTANDING\t")
	for _, rate := range r.Rates {
		fmt.Fprintf(tw, "TKB%d\t%s\t%s\t%d\t%s\t\n", rate.Days, rate.TKB, rate.NPL, rate.DelinquentLoans, rupiah(rate.DelinquentOutstanding))
	}
	tw.Flush()

	fmt.Println()
	tw = tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', tabwriter.AlignRight)
	fmt.Fprintln(tw, "BUCKET\tLOANS\tOUTSTANDING\tSHARE %\t")
	for _, b := range r.Buckets {
		fmt.Fprintf(tw, "%s\t%d\t%s\t%s\t\n", b.Label, b.Loans, rupiah(b.Outstanding), b.Share)
	}
	return tw.Flush()
}

func rupiah(v int64) string {
	s := fmt.Sprintf("%d", v)
	out := make([]byte, 0, len(s)+len(s)/3)
	for i := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, '.')
		}
		out = append(out, s[i])
	}
	return string(out)
}
