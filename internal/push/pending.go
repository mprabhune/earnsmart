package push

import (
	"database/sql"
	"fmt"
	"log"
	"time"
)

// KindPending24h marks the "task has been pending for 24h+" alert.
const KindPending24h = "pending_24h"

// CheckPending finds task logs that have been pending/in-progress for more than
// 24 hours and have not yet triggered a push, sends one push per family parent,
// and records it so it never fires twice.
//
// It runs both on a timer and opportunistically when a parent opens the app, so
// it must be idempotent — the sent_notifications unique constraint guarantees that.
func CheckPending(db *sql.DB, sender *Sender) {
	if !sender.Enabled() {
		return
	}

	rows, err := db.Query(
		`SELECT tl.id, td.family_id, td.title, p.full_name
		   FROM task_logs tl
		   JOIN task_definitions td ON tl.task_definition_id = td.id
		   JOIN profiles p ON tl.assigned_to = p.id
		  WHERE tl.status IN ('pending', 'in_progress')
		    AND tl.created_at < NOW() - INTERVAL '24 hours'
		    AND NOT EXISTS (
		          SELECT 1 FROM sent_notifications sn
		           WHERE sn.task_log_id = tl.id AND sn.kind = $1
		    )`,
		KindPending24h,
	)
	if err != nil {
		log.Printf("push: CheckPending query: %v", err)
		return
	}
	defer rows.Close()

	type pending struct {
		taskLogID string
		familyID  string
		title     string
		kidName   string
	}
	var items []pending
	for rows.Next() {
		var it pending
		if err := rows.Scan(&it.taskLogID, &it.familyID, &it.title, &it.kidName); err != nil {
			continue
		}
		items = append(items, it)
	}

	for _, it := range items {
		// Claim the notification first; if another worker/request beat us to it,
		// ON CONFLICT makes this a no-op and we skip sending.
		res, err := db.Exec(
			`INSERT INTO sent_notifications (task_log_id, kind) VALUES ($1, $2)
			 ON CONFLICT (task_log_id, kind) DO NOTHING`,
			it.taskLogID, KindPending24h,
		)
		if err != nil {
			log.Printf("push: claim sent_notification for %s: %v", it.taskLogID, err)
			continue
		}
		if n, _ := res.RowsAffected(); n == 0 {
			continue
		}

		sender.SendToFamilyParents(it.familyID, Notification{
			Title: "Task still pending",
			Body:  fmt.Sprintf("%s — \"%s\" has been pending for over 24 hours.", it.kidName, it.title),
			URL:   "/",
		})
	}

	if len(items) > 0 {
		log.Printf("push: CheckPending sent %d pending-24h alert(s) at %s", len(items), time.Now().Format(time.RFC3339))
	}
}
