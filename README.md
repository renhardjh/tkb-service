# tkb-service

Calculates **TKB90** (Tingkat Keberhasilan Bayar 90 hari), the repayment success rate that Indonesian P2P lending platforms must publish under OJK rules, plus TKB0, TKB30 and TKB60.

I built the original version of this at KoinWorks for KoinP2P. That code belongs to KoinWorks, so this repository is a clean rewrite of the same idea against the public formula, with a synthetic loan book instead of real data.

## The formula

```
TKB90 = 100% - NPL90
NPL90 = outstanding principal of loans more than 90 days past due / total outstanding principal
```

The same shape gives TKB0, TKB30 and TKB60. A few rules the service applies:

- Only running loans count: disbursed on or before the calculation date, not closed, with principal still outstanding.
- Outstanding means principal only. Interest is not part of the exposure.
- Days past due (DPD) is counted from the due date of the oldest installment that was not fully paid (principal and interest) by the calculation date.
- The threshold is strict. A loan at exactly 90 DPD is still in the healthy part of TKB90; it moves into NPL90 at day 91.
- Everything is evaluated "as of" a date, so repayments made after that date are ignored. This lets you recalculate any past month and get the same number that was published at the time.
- Money is stored as integer rupiah and percentages as hundredths of a percent, rounded half up. No floats anywhere in the calculation.

## Layout

```
cmd/api        HTTP API
cmd/seed       generates a synthetic loan book
cmd/tkb        prints a report in the terminal
internal/loan  loan, installment and repayment model; outstanding and DPD
internal/tkb   TKB/NPL calculation and DPD buckets
internal/store SQLite storage (database/sql, pure Go driver, no CGO)
internal/seed  loan book generator with on-time, late, early payoff and default borrowers
```

## Running it

Requires Go 1.26+.

```bash
make test
make build
./bin/tkb-seed -db tkb.db -loans 5000 -now 2025-10-31
./bin/tkb -db tkb.db -as-of 2025-07-31
```

```
As of 2025-07-31: 2113 running loans, outstanding Rp127.630.915.500

  METRIC  TKB %  NPL %  LOANS     OUTSTANDING
    TKB0  85.47  14.53    316  18.542.446.283
   TKB30  92.19   7.81    166   9.970.512.708
   TKB60  93.70   6.30    136   8.038.271.000
   TKB90  93.98   6.02    126   7.681.745.989

       BUCKET  LOANS      OUTSTANDING  SHARE %
      current   1797  109.088.469.217    85.47
     dpd_1_30    150    8.571.933.575     6.72
    dpd_31_60     30    1.932.241.708     1.51
    dpd_61_90     10      356.525.011     0.28
  dpd_over_90    126    7.681.745.989     6.02
```

Start the API:

```bash
DB_PATH=tkb.db ADDR=:8080 ./bin/tkb-api
```

Or with Docker:

```bash
docker build -t tkb-service .
docker run --rm -p 8080:8080 -v "$PWD/data:/data" tkb-service
```

## API

| Method | Path | Description |
|---|---|---|
| GET | `/healthz` | Database health |
| GET | `/v1/tkb?as_of=2025-07-31` | Live calculation for a date (defaults to today) |
| POST | `/v1/tkb/snapshots` | Calculate and store a closed month, body `{"period":"2025-07"}` |
| GET | `/v1/tkb/snapshots?from=2025-01&to=2025-12` | Stored monthly figures |
| GET | `/v1/tkb/snapshots/{period}` | One month |

Snapshots are taken at the last day of the month and can only be created once the month has ended. Re-running a snapshot for the same period overwrites it, which is how a late correction to repayment data gets republished.

```bash
curl -s 'localhost:8080/v1/tkb?as_of=2025-07-31'
```

```json
{
  "as_of": "2025-07-31",
  "running_loans": 2113,
  "total_outstanding": 127630915500,
  "rates": [
    { "days": 0,  "tkb": 85.47, "npl": 14.53, "delinquent_loans": 316, "delinquent_outstanding": 18542446283 },
    { "days": 30, "tkb": 92.19, "npl": 7.81,  "delinquent_loans": 166, "delinquent_outstanding": 9970512708 },
    { "days": 60, "tkb": 93.70, "npl": 6.30,  "delinquent_loans": 136, "delinquent_outstanding": 8038271000 },
    { "days": 90, "tkb": 93.98, "npl": 6.02,  "delinquent_loans": 126, "delinquent_outstanding": 7681745989 }
  ],
  "buckets": [
    { "label": "current",     "min_dpd": 0,  "max_dpd": 0,    "loans": 1797, "outstanding": 109088469217, "share": 85.47 },
    { "label": "dpd_1_30",    "min_dpd": 1,  "max_dpd": 30,   "loans": 150,  "outstanding": 8571933575,   "share": 6.72 },
    { "label": "dpd_31_60",   "min_dpd": 31, "max_dpd": 60,   "loans": 30,   "outstanding": 1932241708,   "share": 1.51 },
    { "label": "dpd_61_90",   "min_dpd": 61, "max_dpd": 90,   "loans": 10,   "outstanding": 356525011,    "share": 0.28 },
    { "label": "dpd_over_90", "min_dpd": 91, "max_dpd": null, "loans": 126,  "outstanding": 7681745989,   "share": 6.02 }
  ]
}
```

```bash
curl -s -X POST localhost:8080/v1/tkb/snapshots -d '{"period":"2025-07"}'
```

```json
{"period":"2025-07","as_of":"2025-07-31","running_loans":2113,"total_outstanding":127630915500,"tkb0":85.47,"tkb30":92.19,"tkb60":93.70,"tkb90":93.98,"created_at":"2026-11-24T05:02:04Z"}
```

## Storage

SQLite keeps the project easy to run anywhere. The HTTP layer only depends on the `Repository` interface in `internal/httpapi`, and the SQL in `internal/store` is plain enough to move to PostgreSQL by changing the driver and placeholders.
