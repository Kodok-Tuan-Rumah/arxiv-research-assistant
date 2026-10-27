package main

import (
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/malikimayzar/arxiv-research-assistant/internal/api"
	"github.com/malikimayzar/arxiv-research-assistant/internal/client"
	"github.com/malikimayzar/arxiv-research-assistant/internal/logging"
	"github.com/malikimayzar/arxiv-research-assistant/internal/middleware"
	"github.com/malikimayzar/arxiv-research-assistant/internal/repository"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	// Initialize structured logger
	log := logging.NewLogger("arxiv-go-backend", logging.INFO)

	db, err := repository.NewPostgresDB(repository.Config{
		Host:     getEnv("DB_HOST", "localhost"),
		Port:     getEnv("DB_PORT", "5432"),
		User:     getEnv("DB_USER", "arxiv"),
		Password: getEnv("DB_PASSWORD", "arxiv_secret"),
		DBName:   getEnv("DB_NAME", "arxiv_db"),
	})
	if err != nil {
		log.Error("failed to connect to database", logging.Fields{"error": err.Error()})
		os.Exit(1)
	}
	defer db.Close()
	log.Info("Connected to PostgreSQL", logging.Fields{"db_host": getEnv("DB_HOST", "localhost")})

	mlClient := client.NewMLClient(getEnv("ML_SERVICE_URL", "http://localhost:8001"), log)
	if err := mlClient.Ping(); err != nil {
		log.Warning("ML service not reachable", logging.Fields{"error": err.Error(), "ml_url": getEnv("ML_SERVICE_URL", "http://localhost:8001")})
	} else {
		log.Info("Connected to ML service", logging.Fields{"ml_url": getEnv("ML_SERVICE_URL", "http://localhost:8001")})
	}

	app := fiber.New(fiber.Config{
		AppName:      "ArXiv Research Assistant v0.1.0",
		ErrorHandler: api.ErrorHandler,
	})

	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowMethods: "GET,POST,DELETE,OPTIONS",
		AllowHeaders: "Content-Type,Authorization",
	}))
	app.Use(recover.New())
	app.Use(middleware.RequestID())
	app.Use(middleware.Metrics())
	app.Use(logger.New(logger.Config{
		Format: "[${time}] ${status} - ${latency} ${method} ${path} | req-id: ${respHeader:X-Request-ID}\n",
	}))

	app.Get("/health", api.HealthHandler(db, mlClient, api.NewQdrantClient(getEnv("QDRANT_URL", "http://localhost:6333"))))

	paperRepo := repository.NewPaperRepository(db)
	papersHandler := api.NewPapersHandler(paperRepo, mlClient, log)
	app.Get("/papers", papersHandler.List)
	app.Get("/papers/:id", papersHandler.GetByID)
	app.Delete("/papers/:id", papersHandler.Delete)
	logRepo := repository.NewQueryLogRepository(db)

	app.Post("/query", api.QueryHandler(mlClient, logRepo, log))
	app.Get("/query/history", api.QueryHistoryHandler(logRepo))
	app.Get("/metrics", adaptor.HTTPHandler(promhttp.Handler()))

	// Metrics goroutine — stoppable via done channel.
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				middleware.SetActiveGoroutines(float64(runtime.NumGoroutine()))
				middleware.SetPostgresConnections(float64(db.Stats().OpenConnections))
			}
		}
	}()

	// Run server in a goroutine so we can listen for OS signals.
	go func() {
		log.Info("Server starting", logging.Fields{"address": ":8080"})
		if err := app.Listen(":8080"); err != nil && err != http.ErrServerClosed {
			log.Error("server error", logging.Fields{"error": err.Error()})
			os.Exit(1)
		}
	}()

	// Wait for interrupt or terminate signal.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
	log.Info("Shutting down server", logging.Fields{})

	close(done)

	if err := app.ShutdownWithTimeout(10 * time.Second); err != nil {
		log.Error("server shutdown error", logging.Fields{"error": err.Error()})
	}

	log.Info("Server stopped", logging.Fields{})
	log.Info("Active goroutines", logging.Fields{"count": runtime.NumGoroutine()})
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
