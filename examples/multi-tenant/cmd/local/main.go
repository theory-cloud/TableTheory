package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/cors"

	"github.com/theory-cloud/tabletheory"
	"github.com/theory-cloud/tabletheory/examples/multi-tenant/handlers"
	"github.com/theory-cloud/tabletheory/examples/multi-tenant/models"
	"github.com/theory-cloud/tabletheory/pkg/core"
	"github.com/theory-cloud/tabletheory/pkg/session"
)

func main() {
	// Initialize TableTheory
	dbConfig := session.Config{
		Region:   "us-east-1",
		Endpoint: os.Getenv("AWS_ENDPOINT_URL"),
	}

	db, err := theorydb.New(dbConfig)
	if err != nil {
		log.Fatal("Failed to initialize TableTheory:", err)
	}

	// Create tables if running locally
	if os.Getenv("AWS_ENDPOINT_URL") != "" {
		if err := createTables(db); err != nil {
			log.Fatal("Failed to create tables:", err)
		}
	}

	// Initialize handlers
	orgHandler := handlers.NewOrganizationHandler(db)
	userHandler := handlers.NewUserHandler(db)
	projectHandler := handlers.NewProjectHandler(db)
	resourceHandler := handlers.NewResourceHandler(db)
	apiKeyHandler := handlers.NewAPIKeyHandler(db)

	// Setup routes
	r := mux.NewRouter()

	// Health check
	r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}).Methods("GET")

	// Organization routes
	r.HandleFunc("/organizations", orgHandler.CreateOrganization).Methods("POST")
	r.HandleFunc("/organizations", orgHandler.ListOrganizations).Methods("GET")
	r.HandleFunc("/organizations/{org_id}", orgHandler.GetOrganization).Methods("GET")
	r.HandleFunc("/organizations/{org_id}/settings", orgHandler.UpdateOrganizationSettings).Methods("PUT")

	// User routes
	r.HandleFunc("/organizations/{org_id}/invitations", userHandler.InviteUser).Methods("POST")
	r.HandleFunc("/invitations/accept", userHandler.AcceptInvitation).Methods("POST")
	r.HandleFunc("/organizations/{org_id}/users", userHandler.ListUsers).Methods("GET")
	r.HandleFunc("/organizations/{org_id}/users/{user_id}", userHandler.GetUser).Methods("GET")
	r.HandleFunc("/organizations/{org_id}/users/{user_id}", userHandler.UpdateUser).Methods("PUT")
	r.HandleFunc("/organizations/{org_id}/users/{user_id}", userHandler.DeleteUser).Methods("DELETE")

	// Project routes
	r.HandleFunc("/organizations/{org_id}/projects", projectHandler.CreateProject).Methods("POST")
	r.HandleFunc("/organizations/{org_id}/projects", projectHandler.ListProjects).Methods("GET")
	r.HandleFunc("/organizations/{org_id}/projects/{project_id}", projectHandler.GetProject).Methods("GET")
	r.HandleFunc("/organizations/{org_id}/projects/{project_id}", projectHandler.UpdateProject).Methods("PUT")
	r.HandleFunc("/organizations/{org_id}/projects/{project_id}", projectHandler.DeleteProject).Methods("DELETE")

	// Resource tracking routes
	r.HandleFunc("/organizations/{org_id}/resources", resourceHandler.RecordUsage).Methods("POST")
	r.HandleFunc("/organizations/{org_id}/usage", resourceHandler.GetUsageReport).Methods("GET")
	r.HandleFunc("/organizations/{org_id}/projects/{project_id}/usage", resourceHandler.GetProjectUsage).Methods("GET")

	// API key routes
	r.HandleFunc("/organizations/{org_id}/api-keys", apiKeyHandler.CreateAPIKey).Methods("POST")
	r.HandleFunc("/organizations/{org_id}/api-keys", apiKeyHandler.ListAPIKeys).Methods("GET")
	r.HandleFunc("/organizations/{org_id}/api-keys/{key_id}", apiKeyHandler.GetAPIKey).Methods("GET")
	r.HandleFunc("/organizations/{org_id}/api-keys/{key_id}", apiKeyHandler.UpdateAPIKey).Methods("PUT")
	r.HandleFunc("/organizations/{org_id}/api-keys/{key_id}", apiKeyHandler.DeleteAPIKey).Methods("DELETE")

	// Apply middleware
	r.Use(loggingMiddleware)
	r.Use(authMiddleware(db))

	// Setup CORS
	c := cors.New(cors.Options{
		AllowedOrigins:   []string{"http://localhost:3000", "http://localhost:8080"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"*"},
		AllowCredentials: true,
	})

	handler := c.Handler(r)

	// Setup server
	srv := &http.Server{
		Addr:         ":8080",
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server
	go func() {
		fmt.Println("Multi-tenant SaaS API starting on :8080")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("Server failed to start:", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("Shutting down server...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}

	fmt.Println("Server exited")
}

// createTables creates the necessary DynamoDB tables for local development
func createTables(db core.ExtendedDB) error {
	// Register models
	models := []any{
		&models.Organization{},
		&models.User{},
		&models.Project{},
		&models.Resource{},
		&models.APIKey{},
		&models.AuditLog{},
		&models.Invitation{},
		&models.UsageReport{},
	}

	for _, model := range models {
		if err := db.CreateTable(model); err != nil {
			// Ignore if table already exists
			if !isTableExistsError(err) {
				return fmt.Errorf("failed to create table for %T: %w", model, err)
			}
		}
	}

	fmt.Println("Tables created successfully")
	return nil
}

// isTableExistsError checks if the error is due to table already existing
func isTableExistsError(err error) bool {
	return err != nil && err.Error() == "ResourceInUseException"
}

// loggingMiddleware logs all incoming requests
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Create a custom response writer to capture status code
		lrw := &loggingResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(lrw, r)

		duration := time.Since(start)
		log.Printf("%s %s %d %v", r.Method, r.URL.Path, lrw.statusCode, duration)
	})
}

// loggingResponseWriter wraps http.ResponseWriter to capture status code
type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}

// authMiddleware handles authentication for the API
func authMiddleware(db core.ExtendedDB) func(http.Handler) http.Handler {
	apiKeyHandler := handlers.NewAPIKeyHandler(db)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip auth for health check and invitation acceptance
			if r.URL.Path == "/health" || r.URL.Path == "/invitations/accept" {
				next.ServeHTTP(w, r)
				return
			}

			// Check for API key
			apiKey := r.Header.Get("X-API-Key")
			if apiKey != "" {
				// Validate API key
				key, err := apiKeyHandler.ValidateAPIKey(apiKey)
				if err != nil {
					http.Error(w, "Invalid API key", http.StatusUnauthorized)
					return
				}

				// Check rate limit
				if err := apiKeyHandler.CheckRateLimit(key.KeyID); err != nil {
					http.Error(w, err.Error(), http.StatusTooManyRequests)
					return
				}

				// Add key info to context
				ctx := context.WithValue(r.Context(), "api_key_id", key.KeyID)
				ctx = context.WithValue(ctx, "org_id", key.OrgID)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// Check for Bearer token (simplified - in production use proper JWT)
			auth := r.Header.Get("Authorization")
			if auth == "" {
				http.Error(w, "Missing authorization", http.StatusUnauthorized)
				return
			}

			// Extract token
			if len(auth) < 7 || auth[:7] != "Bearer " {
				http.Error(w, "Invalid authorization format", http.StatusUnauthorized)
				return
			}

			token := auth[7:]

			// In production, validate JWT and extract user info
			// For this example, we'll use a simple token format: "user_id:org_id"
			// This is NOT secure and should not be used in production

			// Add user info to context (simplified)
			ctx := context.WithValue(r.Context(), "user_id", "user#"+token)
			ctx = context.WithValue(ctx, "org_id", "org#demo")

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
