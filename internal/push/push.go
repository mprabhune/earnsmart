// Package push delivers Web Push notifications to parent devices.
//
// It is safe to construct a Sender with empty VAPID keys — every send becomes a
// no-op so the rest of the app keeps working when push is not configured.
package push

import (
	"database/sql"
	"encoding/json"
	"io"
	"log"
	"net/http"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/lib/pq"
)

// Notification is the payload delivered to the service worker's `push` handler.
type Notification struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url,omitempty"`
}

type Sender struct {
	db         *sql.DB
	publicKey  string
	privateKey string
	subject    string
}

func NewSender(db *sql.DB, publicKey, privateKey, subject string) *Sender {
	return &Sender{db: db, publicKey: publicKey, privateKey: privateKey, subject: subject}
}

// Enabled reports whether VAPID keys are present.
func (s *Sender) Enabled() bool {
	return s != nil && s.publicKey != "" && s.privateKey != ""
}

// PublicKey is the VAPID application server key the browser needs to subscribe.
func (s *Sender) PublicKey() string {
	if s == nil {
		return ""
	}
	return s.publicKey
}

// SendToProfiles pushes n to every subscription owned by the given profile IDs.
// Dead subscriptions (410 Gone / 404) are pruned.
func (s *Sender) SendToProfiles(profileIDs []string, n Notification) {
	if !s.Enabled() || len(profileIDs) == 0 {
		return
	}

	payload, err := json.Marshal(n)
	if err != nil {
		log.Printf("push: marshal payload: %v", err)
		return
	}

	rows, err := s.db.Query(
		`SELECT id, endpoint, p256dh, auth FROM push_subscriptions WHERE profile_id = ANY($1)`,
		pq.Array(profileIDs),
	)
	if err != nil {
		log.Printf("push: query subscriptions: %v", err)
		return
	}
	defer rows.Close()

	type sub struct {
		id       string
		endpoint string
		p256dh   string
		auth     string
	}
	var subs []sub
	for rows.Next() {
		var sb sub
		if err := rows.Scan(&sb.id, &sb.endpoint, &sb.p256dh, &sb.auth); err != nil {
			continue
		}
		subs = append(subs, sb)
	}

	for _, sb := range subs {
		resp, err := webpush.SendNotification(payload, &webpush.Subscription{
			Endpoint: sb.endpoint,
			Keys:     webpush.Keys{P256dh: sb.p256dh, Auth: sb.auth},
		}, &webpush.Options{
			Subscriber:      s.subject,
			VAPIDPublicKey:  s.publicKey,
			VAPIDPrivateKey: s.privateKey,
			TTL:             86400,
		})
		if err != nil {
			log.Printf("push: send to %s: %v", sb.id, err)
			continue
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

		if resp.StatusCode == http.StatusGone || resp.StatusCode == http.StatusNotFound {
			if _, err := s.db.Exec(`DELETE FROM push_subscriptions WHERE id = $1`, sb.id); err != nil {
				log.Printf("push: prune dead subscription %s: %v", sb.id, err)
			}
		} else if resp.StatusCode >= 300 {
			log.Printf("push: endpoint %s returned status %d", sb.id, resp.StatusCode)
		}
	}
}

// SendToFamilyParents pushes n to every parent profile in the family.
func (s *Sender) SendToFamilyParents(familyID string, n Notification) {
	if !s.Enabled() {
		return
	}
	rows, err := s.db.Query(
		`SELECT id FROM profiles WHERE family_id = $1 AND role = 'parent'`, familyID,
	)
	if err != nil {
		log.Printf("push: query family parents: %v", err)
		return
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	s.SendToProfiles(ids, n)
}
