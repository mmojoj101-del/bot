package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/raghna/fury-sms-gateway/internal/config"
	"github.com/raghna/fury-sms-gateway/internal/domain"
	pgrepo "github.com/raghna/fury-sms-gateway/internal/repository/postgres"
	"github.com/raghna/fury-sms-gateway/pkg/database"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	// CLI flags
	adminEmail := flag.String("admin-email", "", "Super admin email")
	adminPassword := flag.String("admin-password", "", "Super admin password (min 12 chars)")
	adminName := flag.String("admin-name", "", "Super admin name")
	flag.Parse()

	// Fallback to environment variables
	if *adminEmail == "" {
		*adminEmail = os.Getenv("BOOTSTRAP_ADMIN_EMAIL")
	}
	if *adminPassword == "" {
		*adminPassword = os.Getenv("BOOTSTRAP_ADMIN_PASSWORD")
	}
	if *adminName == "" {
		*adminName = os.Getenv("BOOTSTRAP_ADMIN_NAME")
	}

	// Validate
	if *adminEmail == "" || *adminPassword == "" || *adminName == "" {
		fmt.Println("Usage: bootstrap --admin-email=<email> --admin-password=<password> --admin-name=<name>")
		fmt.Println("Or set: BOOTSTRAP_ADMIN_EMAIL, BOOTSTRAP_ADMIN_PASSWORD, BOOTSTRAP_ADMIN_NAME")
		os.Exit(1)
	}

	if len(*adminPassword) < 12 {
		fmt.Println("Error: admin password must be at least 12 characters")
		os.Exit(1)
	}

	// Load config
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Set up logger
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{})))

	ctx := context.Background()

	// Connect to database
	db, err := database.NewPostgresPool(ctx, cfg.Database.URL, 5)
	if err != nil {
		fmt.Printf("Error connecting to database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// Run migrations
	if err := runMigrations(db, cfg.Database.URL); err != nil {
		fmt.Printf("Error running migrations: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✓ Migrations applied successfully")

	// Check if super admin exists
	userRepo := pgrepo.NewUserRepository(db)
	existing, err := userRepo.GetByEmail(ctx, *adminEmail)
	if err == nil && existing != nil {
		if existing.IsSuperAdmin {
			fmt.Printf("Super admin already exists: %s\n", existing.Email)
			return
		}
		// Promote existing user to super admin
		_, err := userRepo.Update(ctx, existing.ID, domain.UpdateUserInput{}, 0)
		if err != nil {
			// Just promote via direct DB
			_, err = db.Exec(ctx,
				"UPDATE users SET is_super_admin = TRUE WHERE id = $1",
				existing.ID,
			)
			if err != nil {
				fmt.Printf("Error promoting user: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("✓ User promoted to super admin: %s\n", existing.Email)
		}
		return
	}

	// Create super admin user
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(*adminPassword), 12)
	if err != nil {
		fmt.Printf("Error hashing password: %v\n", err)
		os.Exit(1)
	}

	var userID string
	err = db.QueryRow(ctx,
		`INSERT INTO users (email, password_hash, name, status, is_super_admin, password_changed_at)
		 VALUES ($1, $2, $3, 'active', TRUE, NOW())
		 RETURNING id`,
		*adminEmail, string(hashedPassword), *adminName,
	).Scan(&userID)
	if err != nil {
		fmt.Printf("Error creating super admin: %v\n", err)
		os.Exit(1)
	}

	// Create default tenant and membership for the super admin
	var tenantID string
	err = db.QueryRow(ctx,
		`INSERT INTO tenants (name, slug, status, settings, created_by, updated_by)
		 VALUES ($1, $2, 'active', '{}'::jsonb, $3, $3)
		 RETURNING id`,
		*adminName+"'s Tenant",
		"default-tenant",
		userID,
	).Scan(&tenantID)
	if err != nil {
		fmt.Printf("Error creating default tenant: %v\n", err)
		os.Exit(1)
	}

	_, err = db.Exec(ctx,
		`INSERT INTO tenant_members (tenant_id, user_id, role)
		 VALUES ($1, $2, 'admin')`,
		tenantID, userID,
	)
	if err != nil {
		fmt.Printf("Error creating tenant membership: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Super admin created: %s (id: %s)\n", *adminEmail, userID)
	fmt.Printf("✓ Default tenant created: %s (id: %s)\n", *adminName+"'s Tenant", tenantID)
	fmt.Println("✓ Bootstrap complete")
}

// runMigrations applies all database migrations.
func runMigrations(db *pgxpool.Pool, dsn string) error {
	_, err := db.Exec(context.Background(), `CREATE EXTENSION IF NOT EXISTS pgcrypto`)
	if err != nil {
		return fmt.Errorf("create extension: %w", err)
	}

	migrationFiles := []string{
		"migrations/001_initial_schema.up.sql",
		"migrations/002_messages.up.sql",
		"migrations/003_outbox.up.sql",
	}

	for i, file := range migrationFiles {
		// Check if this migration already applied by looking for its tables
		if i == 1 {
			var exists bool
			err := db.QueryRow(context.Background(),
				`SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'messages')`,
			).Scan(&exists)
			if err == nil && exists {
				slog.Info("migration 002 already applied, skipping")
				continue
			}
		}
		if i == 2 {
			var exists bool
			err := db.QueryRow(context.Background(),
				`SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'outbox_events')`,
			).Scan(&exists)
			if err == nil && exists {
				slog.Info("migration 003 already applied, skipping")
				continue
			}
		}

		migration, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("read migration file %s: %w", file, err)
		}

		_, err = db.Exec(context.Background(), string(migration))
		if err != nil {
			return fmt.Errorf("execute migration %s: %w", file, err)
		}

		slog.Info("migration applied", "file", file)
	}

	return nil
}
