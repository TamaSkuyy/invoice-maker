package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var db *pgxpool.Pool

// initDB initializes the database connection pool
func initDB() error {
	host := os.Getenv("DB_HOST")
	port := os.Getenv("DB_PORT")
	user := os.Getenv("DB_USER")
	password := os.Getenv("DB_PASSWORD")
	dbname := os.Getenv("DB_NAME")

	// Validate required environment variables
	if host == "" || port == "" || user == "" || dbname == "" {
		return fmt.Errorf("missing required database environment variables")
	}

	// Build connection string
	connString := fmt.Sprintf(
		"postgresql://%s:%s@%s:%s/%s?sslmode=disable",
		user, password, host, port, dbname,
	)

	// Parse connection string and create config
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	config, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return fmt.Errorf("unable to parse database config: %w", err)
	}

	// Configure connection pool settings
	config.MaxConns = 25
	config.MinConns = 5
	config.MaxConnIdleTime = 5 * time.Minute
	config.MaxConnLifetime = 15 * time.Minute

	// Create connection pool
	var dbErr error
	db, dbErr = pgxpool.NewWithConfig(ctx, config)
	if dbErr != nil {
		return fmt.Errorf("unable to create connection pool: %w", dbErr)
	}

	// Test the connection
	if err := db.Ping(ctx); err != nil {
		return fmt.Errorf("unable to ping database: %w", err)
	}

	return nil
}

// closeDB closes the database connection pool gracefully
func closeDB() error {
	if db != nil {
		db.Close()
	}
	return nil
}

// getDB returns the database connection pool
func getDB() *pgxpool.Pool {
	return db
}
