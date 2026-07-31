package models

import (
	"time"

	"github.com/google/uuid"
)

type UserRole string

const (
	RoleParent UserRole = "parent"
	RoleKid    UserRole = "kid"
)

type TaskType string

const (
	TaskTypeAdhoc          TaskType = "adhoc"
	TaskTypeDailyRecurring TaskType = "daily_recurring"
	TaskTypeAccumulation   TaskType = "accumulation"
)

type TaskStatus string

const (
	StatusPending    TaskStatus = "pending"
	StatusInProgress TaskStatus = "in_progress"
	StatusSubmitted  TaskStatus = "submitted"
	StatusApproved   TaskStatus = "approved"
	StatusRejected   TaskStatus = "rejected"
)

type Family struct {
	ID         uuid.UUID `json:"id"`
	FamilyName string    `json:"family_name"`
	CreatedAt  time.Time `json:"created_at"`
}

type Profile struct {
	ID             uuid.UUID `json:"id"`
	FamilyID       uuid.UUID `json:"family_id"`
	FullName       string    `json:"full_name"`
	Email          *string   `json:"email,omitempty"`
	Role           UserRole  `json:"role"`
	PinHash        string    `json:"-"`
	CurrentBalance float64   `json:"current_balance"`
	CreatedAt      time.Time `json:"created_at"`
}

type TaskDefinition struct {
	ID           uuid.UUID  `json:"id"`
	FamilyID     uuid.UUID  `json:"family_id"`
	CreatedBy    *uuid.UUID `json:"created_by,omitempty"`
	Title        string     `json:"title"`
	Description  *string    `json:"description,omitempty"`
	TaskType     TaskType   `json:"task_type"`
	RewardAmount float64    `json:"reward_amount"`
	TargetUnits  int        `json:"target_units"`
	IsActive     bool       `json:"is_active"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type TaskLog struct {
	ID                   uuid.UUID  `json:"id"`
	TaskDefinitionID     uuid.UUID  `json:"task_definition_id"`
	AssignedTo           uuid.UUID  `json:"assigned_to"`
	Status               TaskStatus `json:"status"`
	CurrentProgressUnits int        `json:"current_progress_units"`
	Notes                *string    `json:"notes,omitempty"`
	CompletedAt          *time.Time `json:"completed_at,omitempty"`
	ProofImage           *string    `json:"proof_image,omitempty"`
	SubmittedAt          *time.Time `json:"submitted_at,omitempty"`
	ReviewedAt           *time.Time `json:"reviewed_at,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`

	// Optional joined fields for API responses
	TaskTitle       string   `json:"task_title,omitempty"`
	TaskDescription *string  `json:"task_description,omitempty"`
	TaskType        TaskType `json:"task_type,omitempty"`
	RewardAmount    float64  `json:"reward_amount,omitempty"`
	TargetUnits     int      `json:"target_units,omitempty"`
	AssignedToName  string   `json:"assigned_to_name,omitempty"`
}

type Ledger struct {
	ID              uuid.UUID  `json:"id"`
	FamilyID        uuid.UUID  `json:"family_id"`
	KidID           uuid.UUID  `json:"kid_id"`
	TaskLogID       *uuid.UUID `json:"task_log_id,omitempty"`
	Amount          float64    `json:"amount"`
	TransactionType string     `json:"transaction_type"` // EARNED, PAYOUT, ADJUSTMENT
	CreatedAt       time.Time  `json:"created_at"`
}

// Request / Response Data Structures

type RegisterParentRequest struct {
	FamilyName string `json:"family_name"`
	FullName   string `json:"full_name"`
	Email      string `json:"email"`
	Password   string `json:"password"`
}

type ParentLoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type KidLoginRequest struct {
	FullName string `json:"full_name"`
	PIN      string `json:"pin"`
}

type AuthResponse struct {
	Token string   `json:"token"`
	User  Profile  `json:"user"`
	Family Family  `json:"family"`
}

type CreateKidRequest struct {
	FullName string `json:"full_name"`
	PIN      string `json:"pin"` // 4-digit PIN
}

type CreateTaskRequest struct {
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	TaskType     TaskType `json:"task_type"`
	RewardAmount float64  `json:"reward_amount"`
	TargetUnits  int      `json:"target_units"`
	AssignToKid  *string  `json:"assign_to_kid,omitempty"` // If set, auto-creates task_log
}

type UpdateTaskRequest struct {
	Title        *string   `json:"title"`
	Description  *string   `json:"description"`
	TaskType     *TaskType `json:"task_type"`
	RewardAmount *float64  `json:"reward_amount"`
	TargetUnits  *int      `json:"target_units"`
	IsActive     *bool     `json:"is_active"`
}

type ReviewTaskRequest struct {
	Approved bool    `json:"approved"`
	Notes    *string `json:"notes,omitempty"`
}

type LogProgressRequest struct {
	Units       int        `json:"units"`
	Notes       *string    `json:"notes,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	ProofImage  *string    `json:"proof_image,omitempty"` // base64 data URL
}

type PayoutRequest struct {
	KidID  string  `json:"kid_id"`
	Amount float64 `json:"amount"`
	Notes  *string `json:"notes,omitempty"`
}

type KidSummary struct {
	Profile         Profile `json:"profile"`
	CompletedTasks int     `json:"completed_tasks"`
	PendingTasks   int     `json:"pending_tasks"`
	TotalEarned     float64 `json:"total_earned"`
	CurrentBalance float64 `json:"current_balance"`
}
