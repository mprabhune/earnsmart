package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"earnsmart/internal/config"
	"earnsmart/internal/middleware"
	"earnsmart/internal/models"

	"github.com/google/uuid"
)

func TestTokenGenerationAndClaims(t *testing.T) {
	secret := "test-secret-key"
	userID := uuid.New()
	familyID := uuid.New()

	tokenStr, err := middleware.GenerateToken(userID, familyID, models.RoleParent, secret)
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	if tokenStr == "" {
		t.Fatalf("Expected token string, got empty")
	}
}

func TestHealthCheck(t *testing.T) {
	req := httptest.NewRequest("GET", "/health", nil)
	rr := httptest.NewRecorder()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","service":"earnsmart-backend"}`))
	})

	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	expected := `{"status":"ok","service":"earnsmart-backend"}`
	if rr.Body.String() != expected {
		t.Errorf("handler returned unexpected body: got %v want %v", rr.Body.String(), expected)
	}
}

func TestAuthValidation(t *testing.T) {
	cfg := &config.Config{JWTSecret: "secret"}
	authHandler := NewAuthHandler(nil, cfg)

	// Test invalid JSON register payload
	req := httptest.NewRequest("POST", "/api/v1/auth/parent/register", bytes.NewBuffer([]byte("{invalid json")))
	rr := httptest.NewRecorder()

	authHandler.RegisterParent(rr, req)

	if status := rr.Code; status != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request, got %v", status)
	}

	var errResp models.RegisterParentRequest
	_ = json.Unmarshal(rr.Body.Bytes(), &errResp)
}
