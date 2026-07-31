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
	"golang.org/x/crypto/bcrypt"
)

type ParentHandler struct {
	DB *sql.DB
}

func NewParentHandler(db *sql.DB) *ParentHandler {
	return &ParentHandler{DB: db}
}

// GET /api/v1/parent/tasks
func (h *ParentHandler) GetTasks(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())

	rows, err := h.DB.Query(
		`SELECT id, family_id, created_by, title, description, task_type, reward_amount, target_units, is_active, created_at, updated_at
		 FROM task_definitions
		 WHERE family_id = $1
		 ORDER BY created_at DESC`,
		claims.FamilyID,
	)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "Failed to query tasks")
		return
	}
	defer rows.Close()

	tasks := []models.TaskDefinition{}
	for rows.Next() {
		var t models.TaskDefinition
		if err := rows.Scan(
			&t.ID, &t.FamilyID, &t.CreatedBy, &t.Title, &t.Description, &t.TaskType,
			&t.RewardAmount, &t.TargetUnits, &t.IsActive, &t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			RespondError(w, http.StatusInternalServerError, "Error scanning tasks")
			return
		}
		tasks = append(tasks, t)
	}

	RespondJSON(w, http.StatusOK, tasks)
}

// POST /api/v1/parent/tasks
func (h *ParentHandler) CreateTask(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())

	var req models.CreateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if req.Title == "" || req.RewardAmount < 0 {
		RespondError(w, http.StatusBadRequest, "title is required and reward_amount must be >= 0")
		return
	}

	if req.TargetUnits <= 0 {
		req.TargetUnits = 1
	}
	if req.TaskType == "" {
		req.TaskType = models.TaskTypeAdhoc
	}

	tx, err := h.DB.Begin()
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "Database error")
		return
	}
	defer tx.Rollback()

	var desc *string
	if req.Description != "" {
		desc = &req.Description
	}

	var taskDef models.TaskDefinition
	err = tx.QueryRow(
		`INSERT INTO task_definitions (family_id, created_by, title, description, task_type, reward_amount, target_units, is_active)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, true)
		 RETURNING id, family_id, created_by, title, description, task_type, reward_amount, target_units, is_active, created_at, updated_at`,
		claims.FamilyID, claims.UserID, req.Title, desc, req.TaskType, req.RewardAmount, req.TargetUnits,
	).Scan(
		&taskDef.ID, &taskDef.FamilyID, &taskDef.CreatedBy, &taskDef.Title, &taskDef.Description,
		&taskDef.TaskType, &taskDef.RewardAmount, &taskDef.TargetUnits, &taskDef.IsActive,
		&taskDef.CreatedAt, &taskDef.UpdatedAt,
	)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "Failed to create task definition: "+err.Error())
		return
	}

	// If assigned to a specific kid or all kids in family
	if req.AssignToKid != nil && *req.AssignToKid != "" {
		kidID, err := uuid.Parse(*req.AssignToKid)
		if err == nil {
			_, err = tx.Exec(
				`INSERT INTO task_logs (task_definition_id, assigned_to, status, current_progress_units)
				 VALUES ($1, $2, 'pending', 0)`,
				taskDef.ID, kidID,
			)
			if err != nil {
				RespondError(w, http.StatusInternalServerError, "Failed to assign task to kid")
				return
			}
		}
	} else {
		// Auto-assign to all kids in family if no specific kid specified
		rows, err := tx.Query(`SELECT id FROM profiles WHERE family_id = $1 AND role = 'kid'`, claims.FamilyID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var kID uuid.UUID
				if err := rows.Scan(&kID); err == nil {
					_, _ = tx.Exec(
						`INSERT INTO task_logs (task_definition_id, assigned_to, status, current_progress_units)
						 VALUES ($1, $2, 'pending', 0)`,
						taskDef.ID, kID,
					)
				}
			}
		}
	}

	if err := tx.Commit(); err != nil {
		RespondError(w, http.StatusInternalServerError, "Transaction failed")
		return
	}

	RespondJSON(w, http.StatusCreated, taskDef)
}

// PUT /api/v1/parent/tasks/{id}
func (h *ParentHandler) UpdateTask(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	taskIDStr := chi.URLParam(r, "id")
	taskID, err := uuid.Parse(taskIDStr)
	if err != nil {
		RespondError(w, http.StatusBadRequest, "Invalid task ID")
		return
	}

	var req models.UpdateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	var taskDef models.TaskDefinition
	err = h.DB.QueryRow(
		`SELECT id, family_id, created_by, title, description, task_type, reward_amount, target_units, is_active, created_at, updated_at
		 FROM task_definitions WHERE id = $1 AND family_id = $2`,
		taskID, claims.FamilyID,
	).Scan(
		&taskDef.ID, &taskDef.FamilyID, &taskDef.CreatedBy, &taskDef.Title, &taskDef.Description,
		&taskDef.TaskType, &taskDef.RewardAmount, &taskDef.TargetUnits, &taskDef.IsActive,
		&taskDef.CreatedAt, &taskDef.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			RespondError(w, http.StatusNotFound, "Task not found")
			return
		}
		RespondError(w, http.StatusInternalServerError, "Database error")
		return
	}

	if req.Title != nil {
		taskDef.Title = *req.Title
	}
	if req.Description != nil {
		taskDef.Description = req.Description
	}
	if req.TaskType != nil {
		taskDef.TaskType = *req.TaskType
	}
	if req.RewardAmount != nil {
		taskDef.RewardAmount = *req.RewardAmount
	}
	if req.TargetUnits != nil {
		taskDef.TargetUnits = *req.TargetUnits
	}
	if req.IsActive != nil {
		taskDef.IsActive = *req.IsActive
	}

	err = h.DB.QueryRow(
		`UPDATE task_definitions 
		 SET title = $1, description = $2, task_type = $3, reward_amount = $4, target_units = $5, is_active = $6, updated_at = NOW()
		 WHERE id = $7 AND family_id = $8
		 RETURNING updated_at`,
		taskDef.Title, taskDef.Description, taskDef.TaskType, taskDef.RewardAmount, taskDef.TargetUnits, taskDef.IsActive, taskDef.ID, claims.FamilyID,
	).Scan(&taskDef.UpdatedAt)

	if err != nil {
		RespondError(w, http.StatusInternalServerError, "Failed to update task")
		return
	}

	RespondJSON(w, http.StatusOK, taskDef)
}

// DELETE /api/v1/parent/tasks/{id}
func (h *ParentHandler) DeleteTask(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	taskIDStr := chi.URLParam(r, "id")
	taskID, err := uuid.Parse(taskIDStr)
	if err != nil {
		RespondError(w, http.StatusBadRequest, "Invalid task ID")
		return
	}

	res, err := h.DB.Exec(`DELETE FROM task_definitions WHERE id = $1 AND family_id = $2`, taskID, claims.FamilyID)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "Failed to delete task")
		return
	}

	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		RespondError(w, http.StatusNotFound, "Task not found")
		return
	}

	RespondJSON(w, http.StatusOK, map[string]string{"message": "Task deleted successfully"})
}

// GET /api/v1/parent/approvals
func (h *ParentHandler) GetApprovals(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())

	rows, err := h.DB.Query(
		`SELECT tl.id, tl.task_definition_id, tl.assigned_to, tl.status, tl.current_progress_units, 
		        tl.notes, tl.submitted_at, tl.reviewed_at, tl.created_at,
		        td.title, td.description, td.task_type, td.reward_amount, td.target_units,
		        p.full_name
		 FROM task_logs tl
		 JOIN task_definitions td ON tl.task_definition_id = td.id
		 JOIN profiles p ON tl.assigned_to = p.id
		 WHERE td.family_id = $1 AND tl.status = 'submitted'
		 ORDER BY tl.submitted_at DESC`,
		claims.FamilyID,
	)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "Failed to query pending approvals")
		return
	}
	defer rows.Close()

	logs := []models.TaskLog{}
	for rows.Next() {
		var l models.TaskLog
		if err := rows.Scan(
			&l.ID, &l.TaskDefinitionID, &l.AssignedTo, &l.Status, &l.CurrentProgressUnits,
			&l.Notes, &l.SubmittedAt, &l.ReviewedAt, &l.CreatedAt,
			&l.TaskTitle, &l.TaskDescription, &l.TaskType, &l.RewardAmount, &l.TargetUnits,
			&l.AssignedToName,
		); err != nil {
			RespondError(w, http.StatusInternalServerError, "Error scanning approvals")
			return
		}
		logs = append(logs, l)
	}

	RespondJSON(w, http.StatusOK, logs)
}

// POST /api/v1/parent/approvals/{id}/review
func (h *ParentHandler) ReviewTask(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	logIDStr := chi.URLParam(r, "id")
	logID, err := uuid.Parse(logIDStr)
	if err != nil {
		RespondError(w, http.StatusBadRequest, "Invalid log ID")
		return
	}

	var req models.ReviewTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	tx, err := h.DB.Begin()
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "Database error")
		return
	}
	defer tx.Rollback()

	// Verify task_log belongs to this family and get details
	var kidID uuid.UUID
	var rewardPerUnit float64
	var targetUnits int
	var currentStatus models.TaskStatus
	err = tx.QueryRow(
		`SELECT tl.assigned_to, td.reward_amount, td.target_units, tl.status
		 FROM task_logs tl
		 JOIN task_definitions td ON tl.task_definition_id = td.id
		 WHERE tl.id = $1 AND td.family_id = $2`,
		logID, claims.FamilyID,
	).Scan(&kidID, &rewardPerUnit, &targetUnits, &currentStatus)
	rewardAmount := rewardPerUnit * float64(targetUnits)

	if err != nil {
		if err == sql.ErrNoRows {
			RespondError(w, http.StatusNotFound, "Task submission not found")
			return
		}
		RespondError(w, http.StatusInternalServerError, "Database error")
		return
	}

	if currentStatus == models.StatusApproved {
		RespondError(w, http.StatusBadRequest, "Task has already been approved")
		return
	}

	now := time.Now()
	newStatus := models.StatusApproved
	if !req.Approved {
		newStatus = models.StatusRejected
	}

	// Update task_log
	_, err = tx.Exec(
		`UPDATE task_logs SET status = $1, reviewed_at = $2, notes = COALESCE($3, notes) WHERE id = $4`,
		newStatus, now, req.Notes, logID,
	)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "Failed to update log status")
		return
	}

	if req.Approved {
		// ATOMIC TRANSACTION: Update Kid's Balance & Create Ledger Entry
		_, err = tx.Exec(
			`UPDATE profiles SET current_balance = current_balance + $1 WHERE id = $2`,
			rewardAmount, kidID,
		)
		if err != nil {
			RespondError(w, http.StatusInternalServerError, "Failed to update kid balance")
			return
		}

		_, err = tx.Exec(
			`INSERT INTO ledger (family_id, kid_id, task_log_id, amount, transaction_type)
			 VALUES ($1, $2, $3, $4, 'EARNED')`,
			claims.FamilyID, kidID, logID, rewardAmount,
		)
		if err != nil {
			RespondError(w, http.StatusInternalServerError, "Failed to record ledger entry")
			return
		}
	}

	if err := tx.Commit(); err != nil {
		RespondError(w, http.StatusInternalServerError, "Transaction failed")
		return
	}

	RespondJSON(w, http.StatusOK, map[string]interface{}{
		"message":  "Review processed successfully",
		"approved": req.Approved,
		"status":   newStatus,
	})
}

// GET /api/v1/parent/kids
func (h *ParentHandler) GetKids(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())

	rows, err := h.DB.Query(
		`SELECT id, family_id, full_name, role, current_balance, created_at
		 FROM profiles WHERE family_id = $1 AND role = 'kid'
		 ORDER BY full_name ASC`,
		claims.FamilyID,
	)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "Failed to query kids")
		return
	}
	defer rows.Close()

	kids := []models.Profile{}
	for rows.Next() {
		var p models.Profile
		if err := rows.Scan(&p.ID, &p.FamilyID, &p.FullName, &p.Role, &p.CurrentBalance, &p.CreatedAt); err != nil {
			RespondError(w, http.StatusInternalServerError, "Error scanning kid profiles")
			return
		}
		kids = append(kids, p)
	}

	RespondJSON(w, http.StatusOK, kids)
}

// POST /api/v1/parent/kids
func (h *ParentHandler) CreateKid(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())

	var req models.CreateKidRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if req.FullName == "" || len(req.PIN) != 4 {
		RespondError(w, http.StatusBadRequest, "full_name and a 4-digit PIN are required")
		return
	}

	hashedPIN, err := bcrypt.GenerateFromPassword([]byte(req.PIN), bcrypt.DefaultCost)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "Failed to hash PIN")
		return
	}

	var kid models.Profile
	err = h.DB.QueryRow(
		`INSERT INTO profiles (family_id, full_name, role, pin_hash, current_balance)
		 VALUES ($1, $2, 'kid', $3, 0.00)
		 RETURNING id, family_id, full_name, role, current_balance, created_at`,
		claims.FamilyID, req.FullName, string(hashedPIN),
	).Scan(&kid.ID, &kid.FamilyID, &kid.FullName, &kid.Role, &kid.CurrentBalance, &kid.CreatedAt)

	if err != nil {
		RespondError(w, http.StatusInternalServerError, "Failed to create kid profile")
		return
	}

	RespondJSON(w, http.StatusCreated, kid)
}

// GET /api/v1/parent/summary
func (h *ParentHandler) GetSummary(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())

	// Query each kid in family and compute summary stats
	rows, err := h.DB.Query(
		`SELECT p.id, p.family_id, p.full_name, p.role, p.current_balance, p.created_at,
		        COALESCE(COUNT(CASE WHEN tl.status = 'approved' THEN 1 END), 0) as completed_tasks,
		        COALESCE(COUNT(CASE WHEN tl.status IN ('pending', 'in_progress', 'submitted') THEN 1 END), 0) as pending_tasks,
		        COALESCE(SUM(CASE WHEN l.transaction_type = 'EARNED' THEN l.amount ELSE 0 END), 0) as total_earned
		 FROM profiles p
		 LEFT JOIN task_logs tl ON p.id = tl.assigned_to
		 LEFT JOIN ledger l ON p.id = l.kid_id
		 WHERE p.family_id = $1 AND p.role = 'kid'
		 GROUP BY p.id, p.family_id, p.full_name, p.role, p.current_balance, p.created_at
		 ORDER BY p.full_name ASC`,
		claims.FamilyID,
	)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "Failed to fetch summary: "+err.Error())
		return
	}
	defer rows.Close()

	summaries := []models.KidSummary{}
	for rows.Next() {
		var s models.KidSummary
		if err := rows.Scan(
			&s.Profile.ID, &s.Profile.FamilyID, &s.Profile.FullName, &s.Profile.Role, &s.Profile.CurrentBalance, &s.Profile.CreatedAt,
			&s.CompletedTasks, &s.PendingTasks, &s.TotalEarned,
		); err != nil {
			RespondError(w, http.StatusInternalServerError, "Error scanning summary row")
			return
		}
		s.CurrentBalance = s.Profile.CurrentBalance
		summaries = append(summaries, s)
	}

	RespondJSON(w, http.StatusOK, summaries)
}

// POST /api/v1/parent/payout
func (h *ParentHandler) ProcessPayout(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())

	var req models.PayoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if req.KidID == "" || req.Amount <= 0 {
		RespondError(w, http.StatusBadRequest, "kid_id and positive amount are required")
		return
	}

	kidUUID, err := uuid.Parse(req.KidID)
	if err != nil {
		RespondError(w, http.StatusBadRequest, "Invalid kid_id format")
		return
	}

	tx, err := h.DB.Begin()
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "Database error")
		return
	}
	defer tx.Rollback()

	var currentBalance float64
	err = tx.QueryRow(
		`SELECT current_balance FROM profiles WHERE id = $1 AND family_id = $2 AND role = 'kid'`,
		kidUUID, claims.FamilyID,
	).Scan(&currentBalance)

	if err != nil {
		if err == sql.ErrNoRows {
			RespondError(w, http.StatusNotFound, "Kid profile not found")
			return
		}
		RespondError(w, http.StatusInternalServerError, "Database error")
		return
	}

	if currentBalance < req.Amount {
		RespondError(w, http.StatusBadRequest, "Insufficient kid balance for payout")
		return
	}

	// Deduct balance
	_, err = tx.Exec(
		`UPDATE profiles SET current_balance = current_balance - $1 WHERE id = $2`,
		req.Amount, kidUUID,
	)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "Failed to update kid balance")
		return
	}

	// Record PAYOUT in ledger
	_, err = tx.Exec(
		`INSERT INTO ledger (family_id, kid_id, amount, transaction_type)
		 VALUES ($1, $2, $3, 'PAYOUT')`,
		claims.FamilyID, kidUUID, req.Amount,
	)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "Failed to log payout transaction")
		return
	}

	if err := tx.Commit(); err != nil {
		RespondError(w, http.StatusInternalServerError, "Transaction failed")
		return
	}

	RespondJSON(w, http.StatusOK, map[string]interface{}{
		"message":     "Payout recorded successfully",
		"new_balance": currentBalance - req.Amount,
	})
}
