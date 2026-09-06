// Package exporter produces a self-contained JSON snapshot of a family's data,
// intended for one-time import into the local-first native (Flutter) app.
//
// The snapshot INCLUDES credential hashes (password_hash, pin_hash) so logins
// keep working offline with no reset. It deliberately omits cloud/device-only
// state: reset tokens and Web Push subscriptions.
package exporter

import (
	"database/sql"
	"time"
)

// SchemaVersion of the export envelope. Bump when the shape changes.
const SchemaVersion = 1

type Bundle struct {
	ExportedAt      time.Time        `json:"exported_at"`
	SchemaVersion   int              `json:"schema_version"`
	Family          Family           `json:"family"`
	Profiles        []Profile        `json:"profiles"`
	TaskDefinitions []TaskDefinition `json:"task_definitions"`
	TaskLogs        []TaskLog        `json:"task_logs"`
	Ledger          []LedgerEntry    `json:"ledger"`
}

type Family struct {
	ID         string    `json:"id"`
	FamilyName string    `json:"family_name"`
	CreatedAt  time.Time `json:"created_at"`
}

type Profile struct {
	ID             string    `json:"id"`
	FamilyID       string    `json:"family_id"`
	FullName       string    `json:"full_name"`
	Email          *string   `json:"email"`
	Role           string    `json:"role"`
	PinHash        string    `json:"pin_hash"`
	PasswordHash   *string   `json:"password_hash"`
	CurrentBalance float64   `json:"current_balance"`
	Avatar         *string   `json:"avatar"`
	CreatedAt      time.Time `json:"created_at"`
}

type TaskDefinition struct {
	ID             string     `json:"id"`
	FamilyID       string     `json:"family_id"`
	CreatedBy      *string    `json:"created_by"`
	Title          string     `json:"title"`
	Description    *string    `json:"description"`
	TaskType       string     `json:"task_type"`
	RewardAmount   float64    `json:"reward_amount"`
	TargetUnits    int        `json:"target_units"`
	IsActive       bool       `json:"is_active"`
	ApprovalStatus string     `json:"approval_status"`
	DueDate        *time.Time `json:"due_date"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type TaskLog struct {
	ID                   string     `json:"id"`
	TaskDefinitionID     string     `json:"task_definition_id"`
	AssignedTo           string     `json:"assigned_to"`
	Status               string     `json:"status"`
	CurrentProgressUnits int        `json:"current_progress_units"`
	Notes                *string    `json:"notes"`
	CompletedAt          *time.Time `json:"completed_at"`
	ProofImage           *string    `json:"proof_image"`
	SubmittedAt          *time.Time `json:"submitted_at"`
	ReviewedAt           *time.Time `json:"reviewed_at"`
	CreatedAt            time.Time  `json:"created_at"`
}

type LedgerEntry struct {
	ID              string    `json:"id"`
	FamilyID        string    `json:"family_id"`
	KidID           string    `json:"kid_id"`
	TaskLogID       *string   `json:"task_log_id"`
	Amount          float64   `json:"amount"`
	TransactionType string    `json:"transaction_type"`
	CreatedAt       time.Time `json:"created_at"`
}

// ExportFamily builds a Bundle for a single family.
func ExportFamily(db *sql.DB, familyID string) (*Bundle, error) {
	b := &Bundle{ExportedAt: time.Now().UTC(), SchemaVersion: SchemaVersion}

	if err := db.QueryRow(
		`SELECT id, family_name, created_at FROM families WHERE id = $1`, familyID,
	).Scan(&b.Family.ID, &b.Family.FamilyName, &b.Family.CreatedAt); err != nil {
		return nil, err
	}

	if err := loadProfiles(db, familyID, b); err != nil {
		return nil, err
	}
	if err := loadTaskDefinitions(db, familyID, b); err != nil {
		return nil, err
	}
	if err := loadTaskLogs(db, familyID, b); err != nil {
		return nil, err
	}
	if err := loadLedger(db, familyID, b); err != nil {
		return nil, err
	}
	return b, nil
}

// ListFamilyIDs returns every family id, newest first.
func ListFamilyIDs(db *sql.DB) ([]string, error) {
	rows, err := db.Query(`SELECT id FROM families ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func loadProfiles(db *sql.DB, familyID string, b *Bundle) error {
	rows, err := db.Query(
		`SELECT id, family_id, full_name, email, role, pin_hash, password_hash,
		        current_balance, avatar, created_at
		   FROM profiles WHERE family_id = $1 ORDER BY created_at`,
		familyID,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	b.Profiles = []Profile{}
	for rows.Next() {
		var p Profile
		if err := rows.Scan(
			&p.ID, &p.FamilyID, &p.FullName, &p.Email, &p.Role, &p.PinHash,
			&p.PasswordHash, &p.CurrentBalance, &p.Avatar, &p.CreatedAt,
		); err != nil {
			return err
		}
		b.Profiles = append(b.Profiles, p)
	}
	return rows.Err()
}

func loadTaskDefinitions(db *sql.DB, familyID string, b *Bundle) error {
	rows, err := db.Query(
		`SELECT id, family_id, created_by, title, description, task_type, reward_amount,
		        target_units, is_active, approval_status, due_date, created_at, updated_at
		   FROM task_definitions WHERE family_id = $1 ORDER BY created_at`,
		familyID,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	b.TaskDefinitions = []TaskDefinition{}
	for rows.Next() {
		var t TaskDefinition
		if err := rows.Scan(
			&t.ID, &t.FamilyID, &t.CreatedBy, &t.Title, &t.Description, &t.TaskType,
			&t.RewardAmount, &t.TargetUnits, &t.IsActive, &t.ApprovalStatus, &t.DueDate,
			&t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return err
		}
		b.TaskDefinitions = append(b.TaskDefinitions, t)
	}
	return rows.Err()
}

func loadTaskLogs(db *sql.DB, familyID string, b *Bundle) error {
	rows, err := db.Query(
		`SELECT tl.id, tl.task_definition_id, tl.assigned_to, tl.status,
		        tl.current_progress_units, tl.notes, tl.completed_at, tl.proof_image,
		        tl.submitted_at, tl.reviewed_at, tl.created_at
		   FROM task_logs tl
		   JOIN task_definitions td ON tl.task_definition_id = td.id
		  WHERE td.family_id = $1
		  ORDER BY tl.created_at`,
		familyID,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	b.TaskLogs = []TaskLog{}
	for rows.Next() {
		var l TaskLog
		if err := rows.Scan(
			&l.ID, &l.TaskDefinitionID, &l.AssignedTo, &l.Status, &l.CurrentProgressUnits,
			&l.Notes, &l.CompletedAt, &l.ProofImage, &l.SubmittedAt, &l.ReviewedAt, &l.CreatedAt,
		); err != nil {
			return err
		}
		b.TaskLogs = append(b.TaskLogs, l)
	}
	return rows.Err()
}

func loadLedger(db *sql.DB, familyID string, b *Bundle) error {
	rows, err := db.Query(
		`SELECT id, family_id, kid_id, task_log_id, amount, transaction_type, created_at
		   FROM ledger WHERE family_id = $1 ORDER BY created_at`,
		familyID,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	b.Ledger = []LedgerEntry{}
	for rows.Next() {
		var e LedgerEntry
		if err := rows.Scan(
			&e.ID, &e.FamilyID, &e.KidID, &e.TaskLogID, &e.Amount, &e.TransactionType, &e.CreatedAt,
		); err != nil {
			return err
		}
		b.Ledger = append(b.Ledger, e)
	}
	return rows.Err()
}
