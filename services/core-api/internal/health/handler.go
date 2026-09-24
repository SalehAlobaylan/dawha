package health

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/SalehAlobaylan/dawha/services/core-api/platform/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	Pool *pgxpool.Pool
}

type response struct {
	Status    string    `json:"status"`
	Service   string    `json:"service"`
	Database  string    `json:"database"`
	Timestamp time.Time `json:"timestamp"`
}

func (h Handler) Live(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, response{
		Status:    "ok",
		Service:   "core-api",
		Database:  "not_checked",
		Timestamp: time.Now().UTC(),
	})
}

func (h Handler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 1500*time.Millisecond)
	defer cancel()

	if err := db.Ping(ctx, h.Pool); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, response{
			Status:    "not_ready",
			Service:   "core-api",
			Database:  "unavailable",
			Timestamp: time.Now().UTC(),
		})
		return
	}

	writeJSON(w, http.StatusOK, response{
		Status:    "ready",
		Service:   "core-api",
		Database:  "ready",
		Timestamp: time.Now().UTC(),
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
