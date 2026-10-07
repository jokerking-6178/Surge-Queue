package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/surge/queue-engine/metrics"
	"github.com/surge/queue-engine/queue"
)

// Handlers holds shared dependencies for HTTP route handlers.
type Handlers struct {
	QM *queue.Manager
}

// RegisterRoutes wires all HTTP routes onto the given chi router.
func (h *Handlers) RegisterRoutes(r chi.Router) {
	r.Get("/healthz", h.healthz)
	r.Get("/readyz", h.readyz)

	r.Route("/api/queue", func(r chi.Router) {
		r.Post("/enter", h.enter)
		r.Get("/status/{userId}", h.status)
		r.Post("/heartbeat/{userId}", h.heartbeat)
	})
}

// ---- health ----

func (h *Handlers) healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	writeJSON(w, map[string]string{"status": "ok"})
}

func (h *Handlers) readyz(w http.ResponseWriter, r *http.Request) {
	// In a real deployment, check Redis connectivity here.
	w.WriteHeader(http.StatusOK)
	writeJSON(w, map[string]string{"status": "ready"})
}

// ---- queue endpoints ----

// Enter — POST /api/queue/enter
// Body: { "user_id": "optional" } — if omitted, a UUID is generated.
func (h *Handlers) enter(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer func() {
		metrics.HTTPRequestDuration.WithLabelValues("/api/queue/enter", "POST").
			Observe(time.Since(start).Seconds())
	}()

	var body struct {
		UserID string `json:"user_id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body) // body is optional
	if body.UserID == "" {
		body.UserID = uuid.New().String()
	}

	status, err := h.QM.Enter(r.Context(), body.UserID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]any{
		"user_id": body.UserID,
		"status":  status,
	})
}

// Status — GET /api/queue/status/{userId}
func (h *Handlers) status(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer func() {
		metrics.HTTPRequestDuration.WithLabelValues("/api/queue/status", "GET").
			Observe(time.Since(start).Seconds())
	}()

	userID := chi.URLParam(r, "userId")
	if userID == "" {
		http.Error(w, "missing userId", http.StatusBadRequest)
		return
	}

	status, err := h.QM.Status(r.Context(), userID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, status)
}

// Heartbeat — POST /api/queue/heartbeat/{userId}
func (h *Handlers) heartbeat(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userId")
	if userID == "" {
		http.Error(w, "missing userId", http.StatusBadRequest)
		return
	}

	if err := h.QM.Heartbeat(r.Context(), userID); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

// ---- helpers ----

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
