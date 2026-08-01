package handlers

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

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
	return &AuthHandler{DB: db, Config: cfg}
}

// POST /api/v1/auth/parent/register
func (h *AuthHandler) RegisterParent(w http.ResponseWriter, r *http.Request) {
	var req models.RegisterParentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if req.FamilyName == "" || req.FullName == "" || req.Email == "" {
		RespondError(w, http.StatusBadRequest, "family_name, full_name, and email are required")
		return
	}
	if req.Password == "" && len(req.PIN) != 4 {
		RespondError(w, http.StatusBadRequest, "At least a password or a 4-digit PIN is required")
		return
	}

	tx, err := h.DB.Begin()
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "Database error")
		return
	}
	defer tx.Rollback()

	var family models.Family
	err = tx.QueryRow(
		`INSERT INTO families (family_name) VALUES ($1) RETURNING id, family_name, created_at`,
		req.FamilyName,
	).Scan(&family.ID, &family.FamilyName, &family.CreatedAt)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "Failed to create family")
		return
	}

	// Hash password if provided
	var passwordHash sql.NullString
	if req.Password != "" {
		h, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			RespondError(w, http.StatusInternalServerError, "Failed to hash password")
			return
		}
		passwordHash = sql.NullString{String: string(h), Valid: true}
	}

	// Hash PIN if provided
	var pinHash string
	if len(req.PIN) == 4 {
		h, err := bcrypt.GenerateFromPassword([]byte(req.PIN), bcrypt.DefaultCost)
		if err != nil {
			RespondError(w, http.StatusInternalServerError, "Failed to hash PIN")
			return
		}
		pinHash = string(h)
	}

	var profile models.Profile
	profile.Email = &req.Email
	err = tx.QueryRow(
		`INSERT INTO profiles (family_id, full_name, email, role, pin_hash, password_hash)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, family_id, full_name, email, role, current_balance, created_at`,
		family.ID, req.FullName, req.Email, models.RoleParent, pinHash, passwordHash,
	).Scan(&profile.ID, &profile.FamilyID, &profile.FullName, &profile.Email,
		&profile.Role, &profile.CurrentBalance, &profile.CreatedAt)
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

	RespondJSON(w, http.StatusCreated, models.AuthResponse{Token: token, User: profile, Family: family})
}

// POST /api/v1/auth/parent/login
func (h *AuthHandler) ParentLogin(w http.ResponseWriter, r *http.Request) {
	var req models.ParentLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if req.Email == "" || (req.Password == "" && req.PIN == "") {
		RespondError(w, http.StatusBadRequest, "email and either password or PIN are required")
		return
	}

	var profile models.Profile
	var pinHash string
	var passwordHash sql.NullString
	err := h.DB.QueryRow(
		`SELECT id, family_id, full_name, email, role, pin_hash, password_hash, current_balance, created_at
		 FROM profiles WHERE email = $1 AND role = 'parent'`,
		req.Email,
	).Scan(&profile.ID, &profile.FamilyID, &profile.FullName, &profile.Email,
		&profile.Role, &pinHash, &passwordHash, &profile.CurrentBalance, &profile.CreatedAt)

	if err != nil {
		if err == sql.ErrNoRows {
			RespondError(w, http.StatusUnauthorized, "Invalid credentials")
			return
		}
		RespondError(w, http.StatusInternalServerError, "Database error")
		return
	}

	// Try PIN first, then password
	authenticated := false
	if req.PIN != "" && pinHash != "" {
		authenticated = bcrypt.CompareHashAndPassword([]byte(pinHash), []byte(req.PIN)) == nil
	}
	if !authenticated && req.Password != "" && passwordHash.Valid {
		authenticated = bcrypt.CompareHashAndPassword([]byte(passwordHash.String), []byte(req.Password)) == nil
	}
	if !authenticated {
		RespondError(w, http.StatusUnauthorized, "Invalid credentials")
		return
	}

	var family models.Family
	err = h.DB.QueryRow(`SELECT id, family_name, created_at FROM families WHERE id = $1`, profile.FamilyID).
		Scan(&family.ID, &family.FamilyName, &family.CreatedAt)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "Failed to fetch family")
		return
	}

	token, err := middleware.GenerateToken(profile.ID, profile.FamilyID, models.RoleParent, h.Config.JWTSecret)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "Failed to generate token")
		return
	}

	RespondJSON(w, http.StatusOK, models.AuthResponse{Token: token, User: profile, Family: family})
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

	var matched *models.Profile
	for rows.Next() {
		var p models.Profile
		var pinHash string
		if err := rows.Scan(&p.ID, &p.FamilyID, &p.FullName, &p.Role, &pinHash,
			&p.CurrentBalance, &p.CreatedAt); err != nil {
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

	RespondJSON(w, http.StatusOK, models.AuthResponse{Token: token, User: *matched, Family: family})
}

// POST /api/v1/auth/forgot
func (h *AuthHandler) ForgotCredentials(w http.ResponseWriter, r *http.Request) {
	var req models.ForgotRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	if req.Email == "" {
		RespondError(w, http.StatusBadRequest, "email is required")
		return
	}

	// Check email exists
	var profileID string
	err := h.DB.QueryRow(
		`SELECT id FROM profiles WHERE email = $1 AND role = 'parent'`, req.Email,
	).Scan(&profileID)
	if err != nil {
		// Don't reveal if email exists or not
		RespondJSON(w, http.StatusOK, map[string]string{"message": "If this email is registered, a reset code has been generated."})
		return
	}

	// Generate 6-digit code
	code, err := generateResetCode()
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "Failed to generate reset code")
		return
	}

	// Hash the code before storing
	hashedCode, err := bcrypt.GenerateFromPassword([]byte(code), bcrypt.DefaultCost)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "Failed to process reset code")
		return
	}

	expires := time.Now().Add(15 * time.Minute)
	_, err = h.DB.Exec(
		`UPDATE profiles SET reset_token = $1, reset_token_expires_at = $2 WHERE id = $3`,
		string(hashedCode), expires, profileID,
	)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "Failed to save reset token")
		return
	}

	// Return code directly (family app — no email service)
	RespondJSON(w, http.StatusOK, map[string]string{
		"message":    "Reset code generated. Use it within 15 minutes.",
		"reset_code": code,
	})
}

// POST /api/v1/auth/reset
func (h *AuthHandler) ResetCredentials(w http.ResponseWriter, r *http.Request) {
	var req models.ResetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if req.Email == "" || req.ResetCode == "" {
		RespondError(w, http.StatusBadRequest, "email and reset_code are required")
		return
	}
	if req.NewPassword == "" && len(req.NewPIN) != 4 {
		RespondError(w, http.StatusBadRequest, "Provide new_password or a 4-digit new_pin")
		return
	}

	var profileID string
	var storedToken string
	var tokenExpires time.Time
	err := h.DB.QueryRow(
		`SELECT id, reset_token, reset_token_expires_at FROM profiles
		 WHERE email = $1 AND role = 'parent' AND reset_token IS NOT NULL`,
		req.Email,
	).Scan(&profileID, &storedToken, &tokenExpires)

	if err != nil {
		RespondError(w, http.StatusUnauthorized, "Invalid or expired reset code")
		return
	}

	if time.Now().After(tokenExpires) {
		RespondError(w, http.StatusUnauthorized, "Reset code has expired")
		return
	}

	if bcrypt.CompareHashAndPassword([]byte(storedToken), []byte(req.ResetCode)) != nil {
		RespondError(w, http.StatusUnauthorized, "Invalid reset code")
		return
	}

	// Update password and/or PIN
	var newPasswordHash sql.NullString
	var newPINHash string

	if req.NewPassword != "" {
		h, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
		if err != nil {
			RespondError(w, http.StatusInternalServerError, "Failed to hash password")
			return
		}
		newPasswordHash = sql.NullString{String: string(h), Valid: true}
	}

	if len(req.NewPIN) == 4 {
		h, err := bcrypt.GenerateFromPassword([]byte(req.NewPIN), bcrypt.DefaultCost)
		if err != nil {
			RespondError(w, http.StatusInternalServerError, "Failed to hash PIN")
			return
		}
		newPINHash = string(h)
	}

	_, err = h.DB.Exec(
		`UPDATE profiles
		 SET password_hash = COALESCE(NULLIF($1, ''), password_hash),
		     pin_hash = CASE WHEN $2 != '' THEN $2 ELSE pin_hash END,
		     reset_token = NULL, reset_token_expires_at = NULL
		 WHERE id = $3`,
		newPasswordHash.String, newPINHash, profileID,
	)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "Failed to update credentials")
		return
	}

	RespondJSON(w, http.StatusOK, map[string]string{"message": "Credentials updated successfully. Please log in."})
}

func generateResetCode() (string, error) {
	b := make([]byte, 3)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	// 6-digit numeric code
	code := fmt.Sprintf("%06d", int(b[0])<<16|int(b[1])<<8|int(b[2]))
	if len(code) > 6 {
		code = code[len(code)-6:]
	}
	return code, nil
}
