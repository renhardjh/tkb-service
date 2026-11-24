package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/renhardjh/tkb-service/internal/dates"
	"github.com/renhardjh/tkb-service/internal/loan"
	"github.com/renhardjh/tkb-service/internal/store"
)

func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	healthy := loan.Loan{
		BorrowerID:   1,
		Principal:    900_000,
		RateBPS:      1800,
		TenorMonths:  3,
		DisbursedAt:  dates.MustParse("2025-03-01"),
		Installments: loan.Schedule(900_000, 1800, 3, dates.MustParse("2025-03-01")),
	}
	for i := range healthy.Installments[:2] {
		inst := &healthy.Installments[i]
		inst.Repayments = []loan.Repayment{{Principal: inst.Principal, Interest: inst.Interest, PaidAt: inst.DueDate}}
	}

	defaulted := loan.Loan{
		BorrowerID:   2,
		Principal:    100_000,
		RateBPS:      1800,
		TenorMonths:  1,
		DisbursedAt:  dates.MustParse("2025-03-01"),
		Installments: loan.Schedule(100_000, 1800, 1, dates.MustParse("2025-03-01")),
	}

	if err := db.InsertLoans(ctx, []loan.Loan{healthy, defaulted}); err != nil {
		t.Fatal(err)
	}

	h := New(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	h.today = func() time.Time { return dates.MustParse("2025-10-15") }
	srv := httptest.NewServer(h.Routes())
	t.Cleanup(srv.Close)
	return srv
}

func decode(t *testing.T, res *http.Response, v any) {
	t.Helper()
	defer res.Body.Close()
	if err := json.NewDecoder(res.Body).Decode(v); err != nil {
		t.Fatal(err)
	}
}

func TestReport(t *testing.T) {
	srv := newServer(t)

	res, err := http.Get(srv.URL + "/v1/tkb?as_of=2025-06-30")
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	var body struct {
		RunningLoans     int   `json:"running_loans"`
		TotalOutstanding int64 `json:"total_outstanding"`
		Rates            []struct {
			Days int     `json:"days"`
			TKB  float64 `json:"tkb"`
		} `json:"rates"`
	}
	decode(t, res, &body)

	if body.RunningLoans != 2 || body.TotalOutstanding != 400_000 {
		t.Fatalf("running=%d outstanding=%d", body.RunningLoans, body.TotalOutstanding)
	}
	got := map[int]float64{}
	for _, r := range body.Rates {
		got[r.Days] = r.TKB
	}
	if got[0] != 0 || got[30] != 75 || got[90] != 100 {
		t.Fatalf("rates = %v", got)
	}
}

func TestReportValidation(t *testing.T) {
	srv := newServer(t)
	for _, q := range []string{"as_of=31-07-2025", "as_of=2030-01-01"} {
		res, err := http.Get(srv.URL + "/v1/tkb?" + q)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: status %d", q, res.StatusCode)
		}
	}
}

func TestSnapshotLifecycle(t *testing.T) {
	srv := newServer(t)

	res, err := http.Post(srv.URL+"/v1/tkb/snapshots", "application/json", strings.NewReader(`{"period":"2025-06"}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create status %d", res.StatusCode)
	}
	var snap store.Snapshot
	decode(t, res, &snap)
	if snap.AsOf != "2025-06-30" || snap.TKB90 != 10000 || snap.TKB30 != 7500 {
		t.Fatalf("snapshot = %+v", snap)
	}

	res, err = http.Get(srv.URL + "/v1/tkb/snapshots/2025-06")
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("get status %d", res.StatusCode)
	}
	res.Body.Close()

	res, err = http.Get(srv.URL + "/v1/tkb/snapshots?from=2025-01&to=2025-12")
	if err != nil {
		t.Fatal(err)
	}
	var list struct {
		Data []store.Snapshot `json:"data"`
	}
	decode(t, res, &list)
	if len(list.Data) != 1 {
		t.Fatalf("got %d snapshots", len(list.Data))
	}

	res, err = http.Get(srv.URL + "/v1/tkb/snapshots/2024-01")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("missing snapshot status %d", res.StatusCode)
	}
}

func TestSnapshotRejectsOpenPeriod(t *testing.T) {
	srv := newServer(t)
	res, err := http.Post(srv.URL+"/v1/tkb/snapshots", "application/json", strings.NewReader(`{"period":"2025-10"}`))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status %d", res.StatusCode)
	}
}
