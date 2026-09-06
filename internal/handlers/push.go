package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"earnsmart/internal/middleware"
	"earnsmart/internal/push"
)

type PushHandler struct {
	DB   *sql.DB
	Push *push.Sender
}

func NewPushHandler(db *sql.DB, sender *push.Sender) *PushHandler {
	return &PushHandler{DB: db, Push: sender}
}

// GET /api/v1/parent/push/vapid-key — public VAPID key the browser needs to subscribe.
func (h *PushHandler) VAPIDKey(w http.ResponseWriter, r *http.Request) {
	RespondJSON(w, http.StatusOK, map[string]interface{}{
		"enabled": h.Push.Enabled(),
		"key":     h.Push.PublicKey(),
	})
}

// browserSubscription mirrors the JSON produced by PushSubscription.toJSON().
type browserSubscription struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

// POST /api/v1/parent/push/subscribe — store (or refresh) this device's subscription.
func (h *PushHandler) Subscribe(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())

	var sub browserSubscription
	if err := json.NewDecoder(r.Body).Decode(&sub); err != nil {
		RespondError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	if sub.Endpoint == "" || sub.Keys.P256dh == "" || sub.Keys.Auth == "" {
		RespondError(w, http.StatusBadRequest, "endpoint and keys are required")
		return
	}

	_, err := h.DB.Exec(
		`INSERT INTO push_subscriptions (profile_id, endpoint, p256dh, auth)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (profile_id, endpoint)
		 DO UPDATE SET p256dh = EXCLUDED.p256dh, auth = EXCLUDED.auth`,
		claims.UserID, sub.Endpoint, sub.Keys.P256dh, sub.Keys.Auth,
	)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "Failed to save subscription")
		return
	}

	RespondJSON(w, http.StatusOK, map[string]string{"message": "subscribed"})
}

// POST /api/v1/parent/push/unsubscribe — drop this device's subscription.
func (h *PushHandler) Unsubscribe(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())

	var sub browserSubscription
	if err := json.NewDecoder(r.Body).Decode(&sub); err != nil || sub.Endpoint == "" {
		RespondError(w, http.StatusBadRequest, "endpoint is required")
		return
	}

	if _, err := h.DB.Exec(
		`DELETE FROM push_subscriptions WHERE profile_id = $1 AND endpoint = $2`,
		claims.UserID, sub.Endpoint,
	); err != nil {
		RespondError(w, http.StatusInternalServerError, "Failed to remove subscription")
		return
	}

	RespondJSON(w, http.StatusOK, map[string]string{"message": "unsubscribed"})
}
