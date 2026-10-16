package main

import (
	"log"
	"os"
	"runtime"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/malikimayzar/arxiv-research-assistant/internal/api"
	"github.com/malikimayzar/arxiv-research-assistant/internal/client"
	"github.com/malikimayzar/arxiv-research-assistant/internal/middleware"
	"github.com/malikimayzar/arxiv-research-assistant/internal/repository"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	db, err := repository.NewPostgresDB(repository.Config{
		Host:     getEnv("DB_HOST", "localhost"),
		Port:     getEnv("DB_PORT", "5432"),
		User:     getEnv("DB_USER", "arxiv"),
		Password: getEnv("DB_PASSWORD", "arxiv_secret"),
		DBName:   getEnv("DB_NAME", "arxiv_db"),
	})
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer db.Close()
	log.Println("Connected to PostgreSQL")

	mlClient := client.NewMLClient(getEnv("ML_SERVICE_URL", "http://localhost:8001"))
	if err := mlClient.Ping(); err != nil {
		log.Printf("ML service not reachable: %v", err)
	} else {
		log.Println("Connected to ML service")
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
	papersHandler := api.NewPapersHandler(paperRepo, mlClient)
	app.Get("/papers", papersHandler.List)
	app.Get("/papers/:id", papersHandler.GetByID)
	app.Delete("/papers/:id", papersHandler.Delete)
	// query log repo
	logRepo := repository.NewQueryLogRepository(db)

	app.Post("/query", api.QueryHandler(mlClient, logRepo))
	app.Get("/query/history", api.QueryHistoryHandler(logRepo))
	app.Get("/metrics", adaptor.HTTPHandler(promhttp.Handler()))

	go func() {
		for {
			middleware.SetActiveGoroutines(float64(runtime.NumGoroutine()))
			middleware.SetPostgresConnections(float64(db.Stats().OpenConnections))
			time.Sleep(10 * time.Second)
		}
	}()

	log.Println("Server starting on :8080")
	log.Fatal(app.Listen(":8080"))
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
