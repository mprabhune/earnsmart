package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"earnsmart/internal/config"
	"earnsmart/internal/middleware"
	"earnsmart/internal/models"

	"golang.org/x/crypto/bcrypt"
)

type AuthHandler struct {
	DB     *sql.DB
	Config *config.Config
}

func NewAuthHandler(db *sql.DB, cfg *config.Config) *AuthHandler {
	return &AuthHandler{
		DB:     db,
		Config: cfg,
	}
}

// POST /api/v1/auth/parent/register
func (h *AuthHandler) RegisterParent(w http.ResponseWriter, r *http.Request) {
	var req models.RegisterParentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if req.FamilyName == "" || req.FullName == "" || req.Email == "" || len(req.PIN) != 4 {
		RespondError(w, http.StatusBadRequest, "family_name, full_name, email, and a 4-digit PIN are required")
		return
	}

	tx, err := h.DB.Begin()
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "Database error")
		return
	}
	defer tx.Rollback()

	// Create Family
	var family models.Family
	err = tx.QueryRow(
		`INSERT INTO families (family_name) VALUES ($1) RETURNING id, family_name, created_at`,
		req.FamilyName,
	).Scan(&family.ID, &family.FamilyName, &family.CreatedAt)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "Failed to create family")
		return
	}

	// Hash PIN
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.PIN), bcrypt.DefaultCost)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "Failed to hash PIN")
		return
	}

	// Create Parent Profile
	var profile models.Profile
	profile.Email = &req.Email
	err = tx.QueryRow(
		`INSERT INTO profiles (family_id, full_name, email, role, pin_hash) 
		 VALUES ($1, $2, $3, $4, $5) 
		 RETURNING id, family_id, full_name, email, role, current_balance, created_at`,
		family.ID, req.FullName, req.Email, models.RoleParent, string(hashedPassword),
	).Scan(&profile.ID, &profile.FamilyID, &profile.FullName, &profile.Email, &profile.Role, &profile.CurrentBalance, &profile.CreatedAt)
	if err != nil {
		RespondError(w, http.StatusConflict, "Email already in use or database error")
		return
	}

	if err := tx.Commit(); err != nil {
		RespondError(w, http.StatusInternalServerError, "Transaction commit failed")
		return
	}

	token, err := middleware.GenerateToken(profile.ID, family.ID, models.RoleParent, h.Config.JWTSecret)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "Failed to generate token")
		return
	}

	RespondJSON(w, http.StatusCreated, models.AuthResponse{
		Token:  token,
		User:   profile,
		Family: family,
	})
}

// POST /api/v1/auth/parent/login
func (h *AuthHandler) ParentLogin(w http.ResponseWriter, r *http.Request) {
	var req models.ParentLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if req.Email == "" || len(req.PIN) != 4 {
		RespondError(w, http.StatusBadRequest, "email and 4-digit PIN are required")
		return
	}

	var profile models.Profile
	var pinHash string
	err := h.DB.QueryRow(
		`SELECT id, family_id, full_name, email, role, pin_hash, current_balance, created_at 
		 FROM profiles WHERE email = $1 AND role = 'parent'`,
		req.Email,
	).Scan(&profile.ID, &profile.FamilyID, &profile.FullName, &profile.Email, &profile.Role, &pinHash, &profile.CurrentBalance, &profile.CreatedAt)

	if err != nil {
		if err == sql.ErrNoRows {
			RespondError(w, http.StatusUnauthorized, "Invalid credentials")
			return
		}
		RespondError(w, http.StatusInternalServerError, "Database error")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(pinHash), []byte(req.PIN)); err != nil {
		RespondError(w, http.StatusUnauthorized, "Invalid credentials")
		return
	}

	var family models.Family
	err = h.DB.QueryRow(`SELECT id, family_name, created_at FROM families WHERE id = $1`, profile.FamilyID).
		Scan(&family.ID, &family.FamilyName, &family.CreatedAt)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "Failed to fetch family profile")
		return
	}

	token, err := middleware.GenerateToken(profile.ID, profile.FamilyID, models.RoleParent, h.Config.JWTSecret)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "Failed to generate token")
		return
	}

	RespondJSON(w, http.StatusOK, models.AuthResponse{
		Token:  token,
		User:   profile,
		Family: family,
	})
}

// POST /api/v1/auth/kid/login
func (h *AuthHandler) KidLogin(w http.ResponseWriter, r *http.Request) {
	var req models.KidLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if req.FullName == "" || req.PIN == "" {
		RespondError(w, http.StatusBadRequest, "full_name and pin are required")
		return
	}

	// Fetch all kids with this name (names are not globally unique)
	rows, err := h.DB.Query(
		`SELECT id, family_id, full_name, role, pin_hash, current_balance, created_at
		 FROM profiles WHERE full_name = $1 AND role = 'kid'`,
		req.FullName,
	)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "Database error")
		return
	}
	defer rows.Close()

	// Find the first kid whose PIN matches
	var matched *models.Profile
	for rows.Next() {
		var p models.Profile
		var pinHash string
		if err := rows.Scan(&p.ID, &p.FamilyID, &p.FullName, &p.Role, &pinHash, &p.CurrentBalance, &p.CreatedAt); err != nil {
			continue
		}
		if bcrypt.CompareHashAndPassword([]byte(pinHash), []byte(req.PIN)) == nil {
			matched = &p
			break
		}
	}

	if matched == nil {
		RespondError(w, http.StatusUnauthorized, "Invalid name or PIN")
		return
	}

	var family models.Family
	err = h.DB.QueryRow(`SELECT id, family_name, created_at FROM families WHERE id = $1`, matched.FamilyID).
		Scan(&family.ID, &family.FamilyName, &family.CreatedAt)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "Failed to fetch family info")
		return
	}

	token, err := middleware.GenerateToken(matched.ID, matched.FamilyID, models.RoleKid, h.Config.JWTSecret)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "Failed to generate token")
		return
	}

	RespondJSON(w, http.StatusOK, models.AuthResponse{
		Token:  token,
		User:   *matched,
		Family: family,
	})
}
