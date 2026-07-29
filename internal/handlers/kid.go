package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"earnsmart/internal/middleware"
	"earnsmart/internal/models"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type KidHandler struct {
	DB *sql.DB
}

func NewKidHandler(db *sql.DB) *KidHandler {
	return &KidHandler{DB: db}
}

type KidDashboardResponse struct {
	Kid      models.Profile    `json:"kid"`
	Tasks    []models.TaskLog  `json:"tasks"`
	Ledger   []models.Ledger   `json:"ledger_history"`
}

// GET /api/v1/kid/dashboard
func (h *KidHandler) GetDashboard(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())

	// Fetch kid profile
	var kid models.Profile
	err := h.DB.QueryRow(
		`SELECT id, family_id, full_name, role, current_balance, created_at
		 FROM profiles WHERE id = $1 AND role = 'kid'`,
		claims.UserID,
	).Scan(&kid.ID, &kid.FamilyID, &kid.FullName, &kid.Role, &kid.CurrentBalance, &kid.CreatedAt)

	if err != nil {
		if err == sql.ErrNoRows {
			RespondError(w, http.StatusNotFound, "Kid profile not found")
			return
		}
		RespondError(w, http.StatusInternalServerError, "Database error")
		return
	}

	// Fetch assigned task logs (including definition details)
	rows, err := h.DB.Query(
		`SELECT tl.id, tl.task_definition_id, tl.assigned_to, tl.status, tl.current_progress_units,
		        tl.notes, tl.submitted_at, tl.reviewed_at, tl.created_at,
		        td.title, td.description, td.task_type, td.reward_amount, td.target_units
		 FROM task_logs tl
		 JOIN task_definitions td ON tl.task_definition_id = td.id
		 WHERE tl.assigned_to = $1 AND td.is_active = true
		 ORDER BY tl.created_at DESC`,
		claims.UserID,
	)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "Failed to fetch task logs")
		return
	}
	defer rows.Close()

	tasks := []models.TaskLog{}
	for rows.Next() {
		var l models.TaskLog
		if err := rows.Scan(
			&l.ID, &l.TaskDefinitionID, &l.AssignedTo, &l.Status, &l.CurrentProgressUnits,
			&l.Notes, &l.SubmittedAt, &l.ReviewedAt, &l.CreatedAt,
			&l.TaskTitle, &l.TaskDescription, &l.TaskType, &l.RewardAmount, &l.TargetUnits,
		); err != nil {
			RespondError(w, http.StatusInternalServerError, "Error scanning tasks")
			return
		}
		l.AssignedToName = kid.FullName
		tasks = append(tasks, l)
	}

	// Fetch recent ledger transactions for this kid
	lRows, err := h.DB.Query(
		`SELECT id, family_id, kid_id, task_log_id, amount, transaction_type, created_at
		 FROM ledger WHERE kid_id = $1 ORDER BY created_at DESC LIMIT 20`,
		claims.UserID,
	)
	ledgerHistory := []models.Ledger{}
	if err == nil {
		defer lRows.Close()
		for lRows.Next() {
			var lg models.Ledger
			if err := lRows.Scan(&lg.ID, &lg.FamilyID, &lg.KidID, &lg.TaskLogID, &lg.Amount, &lg.TransactionType, &lg.CreatedAt); err == nil {
				ledgerHistory = append(ledgerHistory, lg)
			}
		}
	}

	RespondJSON(w, http.StatusOK, KidDashboardResponse{
		Kid:    kid,
		Tasks:  tasks,
		Ledger: ledgerHistory,
	})
}

// POST /api/v1/kid/tasks/{id}/log
func (h *KidHandler) LogProgress(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	logIDStr := chi.URLParam(r, "id")
	logID, err := uuid.Parse(logIDStr)
	if err != nil {
		RespondError(w, http.StatusBadRequest, "Invalid task log ID")
		return
	}

	var req models.LogProgressRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Default to +1 unit if body is empty/invalid
		req.Units = 1
	}
	if req.Units <= 0 {
		req.Units = 1
	}

	// Fetch task log and target units
	var currentUnits, targetUnits int
	var status models.TaskStatus
	err = h.DB.QueryRow(
		`SELECT tl.current_progress_units, td.target_units, tl.status
		 FROM task_logs tl
		 JOIN task_definitions td ON tl.task_definition_id = td.id
		 WHERE tl.id = $1 AND tl.assigned_to = $2`,
		logID, claims.UserID,
	).Scan(&currentUnits, &targetUnits, &status)

	if err != nil {
		if err == sql.ErrNoRows {
			RespondError(w, http.StatusNotFound, "Task log not found for this kid")
			return
		}
		RespondError(w, http.StatusInternalServerError, "Database error")
		return
	}

	if status == models.StatusApproved {
		RespondError(w, http.StatusBadRequest, "Cannot log progress on already approved task")
		return
	}

	newUnits := currentUnits + req.Units
	newStatus := models.StatusInProgress
	var submittedAt *time.Time

	// Accumulation Rule: Auto-submit if target reached!
	if newUnits >= targetUnits {
		newUnits = targetUnits
		newStatus = models.StatusSubmitted
		now := time.Now()
		submittedAt = &now
	}

	var notes *string
	if req.Notes != nil && *req.Notes != "" {
		notes = req.Notes
	}

	err = h.DB.QueryRow(
		`UPDATE task_logs
		 SET current_progress_units = $1, status = $2, submitted_at = COALESCE($3, submitted_at), notes = COALESCE($4, notes)
		 WHERE id = $5 AND assigned_to = $6
		 RETURNING status, current_progress_units, submitted_at`,
		newUnits, newStatus, submittedAt, notes, logID, claims.UserID,
	).Scan(&newStatus, &newUnits, &submittedAt)

	if err != nil {
		RespondError(w, http.StatusInternalServerError, "Failed to update task progress")
		return
	}

	RespondJSON(w, http.StatusOK, map[string]interface{}{
		"task_log_id":            logID,
		"current_progress_units": newUnits,
		"target_units":           targetUnits,
		"status":                 newStatus,
		"auto_submitted":         newStatus == models.StatusSubmitted,
	})
}

// POST /api/v1/kid/tasks/{id}/submit
func (h *KidHandler) SubmitTask(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	logIDStr := chi.URLParam(r, "id")
	logID, err := uuid.Parse(logIDStr)
	if err != nil {
		RespondError(w, http.StatusBadRequest, "Invalid task log ID")
		return
	}

	var currentStatus models.TaskStatus
	err = h.DB.QueryRow(
		`SELECT status FROM task_logs WHERE id = $1 AND assigned_to = $2`,
		logID, claims.UserID,
	).Scan(&currentStatus)

	if err != nil {
		if err == sql.ErrNoRows {
			RespondError(w, http.StatusNotFound, "Task log not found for this kid")
			return
		}
		RespondError(w, http.StatusInternalServerError, "Database error")
		return
	}

	if currentStatus == models.StatusApproved {
		RespondError(w, http.StatusBadRequest, "Task is already approved")
		return
	}

	now := time.Now()
	err = h.DB.QueryRow(
		`UPDATE task_logs 
		 SET status = 'submitted', submitted_at = $1 
		 WHERE id = $2 AND assigned_to = $3 
		 RETURNING status`,
		now, logID, claims.UserID,
	).Scan(&currentStatus)

	if err != nil {
		RespondError(w, http.StatusInternalServerError, "Failed to submit task")
		return
	}

	RespondJSON(w, http.StatusOK, map[string]interface{}{
		"message":      "Task submitted to parent for approval",
		"status":       models.StatusSubmitted,
		"submitted_at": now,
	})
}
