package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"log"
	"net/http"

	"earnsmart/internal/config"
	"earnsmart/internal/database"
	"earnsmart/internal/handlers"
	"earnsmart/internal/middleware"
	"earnsmart/web"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

func main() {
	cfg := config.LoadConfig()

	db, err := database.InitDB(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	r := chi.NewRouter()

	// Logger & Recoverer middleware
	r.Use(chiMiddleware.Logger)
	r.Use(chiMiddleware.Recoverer)

	// CORS configuration for web and mobile clients
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	// Web Portal — serves the embedded index.html
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(web.IndexHTML)
	})

	// PWA Manifest (required for Android TWA)
	r.Get("/manifest.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/manifest+json")
		_, _ = w.Write(web.ManifestJSON)
	})

	// Digital Asset Links (required for TWA — no browser bar)
	r.Get("/.well-known/assetlinks.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(web.AssetLinksJSON)
	})

	// App Icons (generated green PNG for TWA)
	icon192 := makeIcon(192)
	icon512 := makeIcon(512)
	r.Get("/icon-192.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(icon192)
	})
	r.Get("/icon-512.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(icon512)
	})

	// Health Check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","service":"earnsmart-backend"}`))
	})

	// Handlers
	authHandler := handlers.NewAuthHandler(db, cfg)
	parentHandler := handlers.NewParentHandler(db)
	kidHandler := handlers.NewKidHandler(db)

	// Public Auth API Routes
	r.Route("/api/v1/auth", func(r chi.Router) {
		r.Post("/parent/register", authHandler.RegisterParent)
		r.Post("/parent/login", authHandler.ParentLogin)
		r.Post("/kid/login", authHandler.KidLogin)
		r.Post("/forgot", authHandler.ForgotCredentials)
		r.Post("/reset", authHandler.ResetCredentials)
	})

	// Protected Routes (JWT required)
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(middleware.AuthMiddleware(cfg.JWTSecret))

		// Parent Endpoints
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
			r.Get("/kids/{id}/tasks", parentHandler.GetKidTasks)
			r.Post("/payout", parentHandler.ProcessPayout)
			r.Get("/notifications", parentHandler.GetNotifications)

			r.Patch("/profile/avatar", parentHandler.UpdateAvatar)
			r.Patch("/kids/{id}/avatar", parentHandler.UpdateKidAvatar)
		})

		// Kid Endpoints
		r.Route("/kid", func(r chi.Router) {
			r.Use(middleware.RequireRole("kid"))

			r.Get("/dashboard", kidHandler.GetDashboard)
			r.Patch("/profile/avatar", kidHandler.UpdateAvatar)
			r.Post("/tasks/{id}/log", kidHandler.LogProgress)
			r.Post("/tasks/{id}/submit", kidHandler.SubmitTask)
		})
	})

	log.Printf("EarnSmart Backend API starting on port :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, r); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

// makeIcon generates a solid green PNG icon of the given size
func makeIcon(size int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	green := color.RGBA{R: 26, G: 107, B: 60, A: 255} // #1a6b3c
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.Set(x, y, green)
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}
