package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/renhardjh/tkb-service/internal/dates"
	"github.com/renhardjh/tkb-service/internal/seed"
	"github.com/renhardjh/tkb-service/internal/store"
)

func main() {
	var (
		dbPath = flag.String("db", "tkb.db", "sqlite database path")
		count  = flag.Int("loans", 5000, "number of loans to generate")
		from   = flag.String("from", "2024-01-01", "first disbursement date")
		to     = flag.String("to", "2025-09-30", "last disbursement date")
		now    = flag.String("now", dates.Format(dates.Today()), "repayments after this date are not generated")
		rseed  = flag.Int64("seed", 42, "random seed")
		reset  = flag.Bool("reset", true, "delete existing data first")
	)
	flag.Parse()

	if err := run(*dbPath, *count, *from, *to, *now, *rseed, *reset); err != nil {
		fmt.Fprintln(os.Stderr, "seed:", err)
		os.Exit(1)
	}
}

func run(dbPath string, count int, from, to, now string, rseed int64, reset bool) error {
	cfg := seed.Config{Loans: count, Seed: rseed}
	var err error
	if cfg.From, err = dates.Parse(from); err != nil {
		return err
	}
	if cfg.To, err = dates.Parse(to); err != nil {
		return err
	}
	if cfg.Now, err = dates.Parse(now); err != nil {
		return err
	}
	if cfg.To.Before(cfg.From) {
		return fmt.Errorf("-to must not be before -from")
	}

	ctx := context.Background()
	db, err := store.Open(ctx, dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	if reset {
		if err := db.Reset(ctx); err != nil {
			return err
		}
	}

	start := time.Now()
	loans := seed.Generate(cfg)
	if err := db.InsertLoans(ctx, loans); err != nil {
		return err
	}
	fmt.Printf("inserted %d loans into %s in %s\n", len(loans), dbPath, time.Since(start).Round(time.Millisecond))
	return nil
}
