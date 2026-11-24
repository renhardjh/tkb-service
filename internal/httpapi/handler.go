package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/renhardjh/tkb-service/internal/dates"
	"github.com/renhardjh/tkb-service/internal/loan"
	"github.com/renhardjh/tkb-service/internal/store"
	"github.com/renhardjh/tkb-service/internal/tkb"
)

type Repository interface {
	Ping(ctx context.Context) error
	LoansAsOf(ctx context.Context, asOf time.Time) ([]loan.Loan, error)
	SaveSnapshot(ctx context.Context, period string, r tkb.Report) (store.Snapshot, error)
	Snapshots(ctx context.Context, from, to string) ([]store.Snapshot, error)
	Snapshot(ctx context.Context, period string) (store.Snapshot, error)
}

type Handler struct {
	repo  Repository
	log   *slog.Logger
	today func() time.Time
}

func New(repo Repository, log *slog.Logger) *Handler {
	return &Handler{repo: repo, log: log, today: dates.Today}
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.health)
	mux.HandleFunc("GET /v1/tkb", h.report)
	mux.HandleFunc("GET /v1/tkb/snapshots", h.listSnapshots)
	mux.HandleFunc("GET /v1/tkb/snapshots/{period}", h.getSnapshot)
	mux.HandleFunc("POST /v1/tkb/snapshots", h.createSnapshot)
	return h.logRequests(mux)
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	if err := h.repo.Ping(r.Context()); err != nil {
		writeError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) report(w http.ResponseWriter, r *http.Request) {
	asOf := h.today()
	if v := r.URL.Query().Get("as_of"); v != "" {
		t, err := dates.Parse(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		asOf = t
	}
	if asOf.After(h.today()) {
		writeError(w, http.StatusBadRequest, "as_of cannot be in the future")
		return
	}

	loans, err := h.repo.LoansAsOf(r.Context(), asOf)
	if err != nil {
		h.internal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, tkb.Calculate(loans, asOf))
}

type snapshotRequest struct {
	Period string `json:"period"`
}

func (h *Handler) createSnapshot(w http.ResponseWriter, r *http.Request) {
	var req snapshotRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	month, err := dates.ParsePeriod(req.Period)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	asOf := dates.EndOfMonth(month)
	if !asOf.Before(h.today()) {
		writeError(w, http.StatusUnprocessableEntity, "period has not closed yet")
		return
	}

	loans, err := h.repo.LoansAsOf(r.Context(), asOf)
	if err != nil {
		h.internal(w, r, err)
		return
	}
	snap, err := h.repo.SaveSnapshot(r.Context(), req.Period, tkb.Calculate(loans, asOf))
	if err != nil {
		h.internal(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, snap)
}

func (h *Handler) listSnapshots(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	from, to := q.Get("from"), q.Get("to")
	if from == "" {
		from = "0000-01"
	}
	if to == "" {
		to = "9999-12"
	}
	for _, p := range []string{from, to} {
		if _, err := dates.ParsePeriod(p); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	snaps, err := h.repo.Snapshots(r.Context(), from, to)
	if err != nil {
		h.internal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": snaps})
}

func (h *Handler) getSnapshot(w http.ResponseWriter, r *http.Request) {
	period := r.PathValue("period")
	if _, err := dates.ParsePeriod(period); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	snap, err := h.repo.Snapshot(r.Context(), period)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "snapshot not found")
		return
	}
	if err != nil {
		h.internal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

func (h *Handler) internal(w http.ResponseWriter, r *http.Request, err error) {
	h.log.ErrorContext(r.Context(), "request failed", "method", r.Method, "path", r.URL.Path, "err", err)
	writeError(w, http.StatusInternalServerError, "internal server error")
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (h *Handler) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		h.log.Info("http", "method", r.Method, "path", r.URL.Path, "status", rec.status, "duration", time.Since(start).String())
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
