package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"earnsmart/internal/config"
	"earnsmart/internal/database"
	"earnsmart/internal/handlers"
	"earnsmart/internal/middleware"
	"earnsmart/internal/models"

	"github.com/go-chi/chi/v5"
)

func setupTestRouter(t *testing.T) (http.Handler, string) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://localhost:5432/earnsmart?sslmode=disable"
	}

	db, err := database.InitDB(dbURL)
	if err != nil {
		t.Skipf("Skipping integration test; postgres connection failed: %v", err)
	}

	cfg := &config.Config{
		Port:        "8080",
		DatabaseURL: dbURL,
		JWTSecret:   "test-jwt-secret-key-12345",
	}

	r := chi.NewRouter()

	authHandler := handlers.NewAuthHandler(db, cfg)
	parentHandler := handlers.NewParentHandler(db)
	kidHandler := handlers.NewKidHandler(db)

	r.Route("/api/v1/auth", func(r chi.Router) {
		r.Post("/parent/register", authHandler.RegisterParent)
		r.Post("/parent/login", authHandler.ParentLogin)
		r.Post("/kid/login", authHandler.KidLogin)
	})

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(middleware.AuthMiddleware(cfg.JWTSecret))

		r.Route("/parent", func(r chi.Router) {
			r.Use(middleware.RequireRole("parent"))
			r.Get("/tasks", parentHandler.GetTasks)
			r.Post("/tasks", parentHandler.CreateTask)
			r.Put("/tasks/{id}", parentHandler.UpdateTask)
			r.Delete("/tasks/{id}", parentHandler.DeleteTask)
			r.Get("/approvals", parentHandler.GetApprovals)
			r.Post("/approvals/{id}/review", parentHandler.ReviewTask)
			r.Get("/summary", parentHandler.GetSummary)
			r.Get("/kids", parentHandler.GetKids)
			r.Post("/kids", parentHandler.CreateKid)
			r.Post("/payout", parentHandler.ProcessPayout)
		})

		r.Route("/kid", func(r chi.Router) {
			r.Use(middleware.RequireRole("kid"))
			r.Get("/dashboard", kidHandler.GetDashboard)
			r.Post("/tasks/{id}/log", kidHandler.LogProgress)
			r.Post("/tasks/{id}/submit", kidHandler.SubmitTask)
		})
	})

	return r, dbURL
}

func TestFullIntegrationWorkflow(t *testing.T) {
	router, _ := setupTestRouter(t)

	// 1. Register Parent & Family
	parentRegPayload := models.RegisterParentRequest{
		FamilyName: "Smith Household",
		FullName:   "Alice Smith",
		Email:      "alice@smithfamily.test",
		Password:   "ParentPassword123!",
	}
	bodyBytes, _ := json.Marshal(parentRegPayload)

	req := httptest.NewRequest("POST", "/api/v1/auth/parent/register", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated && w.Code != http.StatusConflict {
		t.Fatalf("Register parent failed: code %d, body %s", w.Code, w.Body.String())
	}

	// 2. Parent Login
	loginPayload := models.ParentLoginRequest{
		Email:    "alice@smithfamily.test",
		Password: "ParentPassword123!",
	}
	bodyBytes, _ = json.Marshal(loginPayload)
	req = httptest.NewRequest("POST", "/api/v1/auth/parent/login", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Parent login failed: code %d, body %s", w.Code, w.Body.String())
	}

	var authResp models.AuthResponse
	_ = json.Unmarshal(w.Body.Bytes(), &authResp)
	parentToken := authResp.Token

	if parentToken == "" {
		t.Fatalf("Expected parent token, got empty")
	}

	// 3. Create Kid Profile
	createKidPayload := models.CreateKidRequest{
		FullName: "Bobby Smith",
		PIN:      "1234",
	}
	bodyBytes, _ = json.Marshal(createKidPayload)
	req = httptest.NewRequest("POST", "/api/v1/parent/kids", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+parentToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("Create kid profile failed: code %d, body %s", w.Code, w.Body.String())
	}

	var kidProfile models.Profile
	_ = json.Unmarshal(w.Body.Bytes(), &kidProfile)

	// 4. Kid Login via 4-Digit PIN
	kidLoginPayload := models.KidLoginRequest{
		KidID: kidProfile.ID.String(),
		PIN:   "1234",
	}
	bodyBytes, _ = json.Marshal(kidLoginPayload)
	req = httptest.NewRequest("POST", "/api/v1/auth/kid/login", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Kid login failed: code %d, body %s", w.Code, w.Body.String())
	}

	var kidAuthResp models.AuthResponse
	_ = json.Unmarshal(w.Body.Bytes(), &kidAuthResp)
	kidToken := kidAuthResp.Token

	// 5. Parent creates accumulation task definition ("Complete 5 math pages", target: 5 units, $2.50)
	kidIDStr := kidProfile.ID.String()
	createTaskPayload := models.CreateTaskRequest{
		Title:        "Complete 5 math pages",
		Description:  "Math practice workbook",
		TaskType:     models.TaskTypeAccumulation,
		RewardAmount: 2.50,
		TargetUnits:  5,
		AssignToKid:  &kidIDStr,
	}
	bodyBytes, _ = json.Marshal(createTaskPayload)
	req = httptest.NewRequest("POST", "/api/v1/parent/tasks", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+parentToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("Create task failed: code %d, body %s", w.Code, w.Body.String())
	}

	// 6. Kid Dashboard
	req = httptest.NewRequest("GET", "/api/v1/kid/dashboard", nil)
	req.Header.Set("Authorization", "Bearer "+kidToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Kid dashboard failed: code %d, body %s", w.Code, w.Body.String())
	}

	var kidDash handlers.KidDashboardResponse
	_ = json.Unmarshal(w.Body.Bytes(), &kidDash)

	if len(kidDash.Tasks) == 0 {
		t.Fatalf("Expected assigned task in kid dashboard, found 0")
	}
	taskLogID := kidDash.Tasks[0].ID.String()

	// 7. Kid logs progress (5 units) -> Auto-submits task
	logPayload := models.LogProgressRequest{
		Units: 5,
	}
	bodyBytes, _ = json.Marshal(logPayload)
	req = httptest.NewRequest("POST", "/api/v1/kid/tasks/"+taskLogID+"/log", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+kidToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Kid log progress failed: code %d, body %s", w.Code, w.Body.String())
	}

	// 8. Parent fetches approvals
	req = httptest.NewRequest("GET", "/api/v1/parent/approvals", nil)
	req.Header.Set("Authorization", "Bearer "+parentToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Parent approvals failed: code %d, body %s", w.Code, w.Body.String())
	}

	var approvals []models.TaskLog
	_ = json.Unmarshal(w.Body.Bytes(), &approvals)
	if len(approvals) == 0 {
		t.Fatalf("Expected pending approval, found 0")
	}

	// 9. Parent Reviews & Approves task -> Balance should increase by $2.50
	reviewPayload := models.ReviewTaskRequest{
		Approved: true,
	}
	bodyBytes, _ = json.Marshal(reviewPayload)
	req = httptest.NewRequest("POST", "/api/v1/parent/approvals/"+taskLogID+"/review", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+parentToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Parent review failed: code %d, body %s", w.Code, w.Body.String())
	}

	// 10. Parent checks summary
	req = httptest.NewRequest("GET", "/api/v1/parent/summary", nil)
	req.Header.Set("Authorization", "Bearer "+parentToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Parent summary failed: code %d, body %s", w.Code, w.Body.String())
	}

	var summaries []models.KidSummary
	_ = json.Unmarshal(w.Body.Bytes(), &summaries)
	if len(summaries) == 0 || summaries[0].CurrentBalance < 2.50 {
		t.Fatalf("Expected balance >= 2.50, got %v", summaries)
	}

	// 11. Parent processes Payout ($1.00)
	payoutPayload := models.PayoutRequest{
		KidID:  kidProfile.ID.String(),
		Amount: 1.00,
	}
	bodyBytes, _ = json.Marshal(payoutPayload)
	req = httptest.NewRequest("POST", "/api/v1/parent/payout", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+parentToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Payout failed: code %d, body %s", w.Code, w.Body.String())
	}
}
