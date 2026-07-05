package router

import (
	"log/slog"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/raghna/fury-sms-gateway/internal/api/handler"
	"github.com/raghna/fury-sms-gateway/internal/api/middleware"
	"github.com/raghna/fury-sms-gateway/internal/config"
	"github.com/raghna/fury-sms-gateway/internal/connector"
	"github.com/raghna/fury-sms-gateway/internal/domain"
	"github.com/raghna/fury-sms-gateway/internal/event"
	pgrepo "github.com/raghna/fury-sms-gateway/internal/repository/postgres"
	"github.com/raghna/fury-sms-gateway/internal/service"
)

// New creates and configures a new Fiber application with all routes.
func New(
	cfg *config.Config,
	db *pgxpool.Pool,
	rdb *redis.Client,
	eventBus event.Bus,
	clock domain.Clock,
	workers handler.WorkerHealthChecker,
) (*fiber.App, error) {
	app := fiber.New(fiber.Config{
		ReadTimeout:  cfg.HTTP.ReadTimeout,
		WriteTimeout: cfg.HTTP.WriteTimeout,
		AppName:      cfg.App.Name,
	})

	// ============================================================
	// Initialize repositories
	// ============================================================
	userRepo := pgrepo.NewUserRepository(db)
	tenantRepo := pgrepo.NewTenantRepository(db)
	memberRepo := pgrepo.NewTenantMemberRepository(db)
	apiKeyRepo := pgrepo.NewAPIKeyRepository(db)
	connectorRepo := pgrepo.NewConnectorRepository(db)
	routeRepo := pgrepo.NewRouteRepository(db)
	msgRepo := pgrepo.NewMessageRepository(db)
	auditRepo := pgrepo.NewAuditLogRepository(db)
	refreshTokenRepo := pgrepo.NewRefreshTokenRepository(db)
	txManager := pgrepo.NewTxManager(db)

	// ============================================================
	// Initialize services
	// ============================================================
	authService := service.NewAuthService(
		userRepo, memberRepo, apiKeyRepo, refreshTokenRepo, auditRepo,
		eventBus, &cfg.JWT, clock,
	)
	tenantService := service.NewTenantService(tenantRepo, memberRepo, auditRepo, eventBus, clock)
	memberService := service.NewMemberService(memberRepo, auditRepo, eventBus, clock)
	apiKeyService := service.NewAPIKeyService(apiKeyRepo, auditRepo, eventBus, cfg.JWT.Secret, clock)
	connectorService := service.NewConnectorService(connectorRepo, auditRepo, txManager, eventBus, clock)
	routeService := service.NewRouteService(routeRepo, connectorRepo, auditRepo, txManager, eventBus, clock)
	auditService := service.NewAuditService(auditRepo)

	// ============================================================
	// Initialize handlers
	// ============================================================
	healthHandler := handler.NewHealthHandler(db, rdb, workers)
	authHandler := handler.NewAuthHandler(authService)
	tenantHandler := handler.NewTenantHandler(tenantService)
	memberHandler := handler.NewMemberHandler(memberService)
	apiKeyHandler := handler.NewAPIKeyHandler(apiKeyService)
	connectorHandler := handler.NewConnectorHandler(connectorService)
	routeHandler := handler.NewRouteHandler(routeService)
	auditHandler := handler.NewAuditHandler(auditService)

	// DLR handler (for delivery receipts)
	dlrMapper := connector.NewDefaultDLRMapper(domain.ConnectorTypeHTTPClient)
	noopMetrics := connector.NewNoopMetricsRecorder()
	dlrHandler := handler.NewDLRHandler(msgRepo, connectorRepo, dlrMapper, noopMetrics)

	// ============================================================
	// Initialize middleware
	// ============================================================
	rbacMiddleware := middleware.NewRBACMiddleware()

	// ============================================================
	// Global middleware
	// ============================================================
	app.Use(middleware.RequestID())
	app.Use(middleware.Logger())
	app.Use(cors.New(cors.Config{
		AllowOrigins:     joinStrings(cfg.CORS.AllowedOrigins),
		AllowMethods:     joinStrings(cfg.CORS.AllowedMethods),
		AllowHeaders:     joinStrings(cfg.CORS.AllowedHeaders),
		AllowCredentials: cfg.CORS.AllowCredentials,
		MaxAge:           cfg.CORS.MaxAge,
	}))
	app.Use(middleware.TenantContext())
	app.Use(middleware.MetricsMiddleware())

	// ============================================================
	// Health & Metrics (no auth required)
	// ============================================================
	app.Get("/health", healthHandler.Health)
	app.Get("/ready", healthHandler.Readiness)
	app.Get("/metrics", handler.MetricsHandler())

	// ============================================================
	// API v1 routes
	// ============================================================
	v1 := app.Group("/api/v1")

	// Auth routes (no JWT required)
	auth := v1.Group("/auth")
	auth.Post("/login", authHandler.Login)
	auth.Post("/refresh", authHandler.RefreshToken)
	auth.Post("/logout", authHandler.Logout)

	// Protected routes (JWT required)
	protected := v1.Group("")
	protected.Use(middleware.JWTAuth(cfg.JWT.Secret))

	// User profile
	protected.Get("/me", authHandler.Me)
	protected.Post("/switch-tenant/:tenantID", authHandler.SwitchTenant)

	// Tenant management (admin+ only)
	protected.Post("/tenants", rbacMiddleware.RequireRole(domain.MemberRoleAdmin), tenantHandler.Create)
	protected.Get("/tenants", rbacMiddleware.RequireRole(domain.MemberRoleAdmin), tenantHandler.List)
	protected.Get("/tenants/:id", tenantHandler.GetByID)
	protected.Put("/tenants/:id", rbacMiddleware.RequireRole(domain.MemberRoleAdmin), tenantHandler.Update)
	protected.Delete("/tenants/:id", rbacMiddleware.RequireRole(domain.MemberRoleAdmin), tenantHandler.Delete)

	// Tenant members
	protected.Post("/tenants/:tenantID/members", rbacMiddleware.RequireRole(domain.MemberRoleAdmin), memberHandler.Add)
	protected.Get("/tenants/:tenantID/members", memberHandler.ListByTenant)
	protected.Get("/tenants/:tenantID/members/:userID", memberHandler.UpdateRole)
	protected.Put("/tenants/:tenantID/members/:userID", rbacMiddleware.RequireRole(domain.MemberRoleAdmin), memberHandler.UpdateRole)
	protected.Delete("/tenants/:tenantID/members/:userID", rbacMiddleware.RequireRole(domain.MemberRoleAdmin), memberHandler.Remove)

	// API Keys (tenant-scoped)
	protected.Post("/api-keys", rbacMiddleware.RequireRole(domain.MemberRoleAdmin), apiKeyHandler.Create)
	protected.Get("/api-keys", apiKeyHandler.ListByTenant)
	protected.Get("/api-keys/:id", apiKeyHandler.GetByID)
	protected.Put("/api-keys/:id", rbacMiddleware.RequireRole(domain.MemberRoleAdmin), apiKeyHandler.Update)
	protected.Delete("/api-keys/:id", rbacMiddleware.RequireRole(domain.MemberRoleAdmin), apiKeyHandler.Delete)

	// Connectors (tenant-scoped)
	protected.Post("/connectors", rbacMiddleware.RequireRole(domain.MemberRoleAdmin), connectorHandler.Create)
	protected.Get("/connectors", connectorHandler.ListByTenant)
	protected.Get("/connectors/:id", connectorHandler.GetByID)
	protected.Put("/connectors/:id", rbacMiddleware.RequireRole(domain.MemberRoleAdmin), connectorHandler.Update)
	protected.Delete("/connectors/:id", rbacMiddleware.RequireRole(domain.MemberRoleAdmin), connectorHandler.Delete)
	protected.Post("/connectors/:id/test", rbacMiddleware.RequireRole(domain.MemberRoleAdmin, domain.MemberRoleOperator), connectorHandler.TestConnection)

	// Routes (tenant-scoped)
	protected.Post("/routes", rbacMiddleware.RequireRole(domain.MemberRoleAdmin), routeHandler.Create)
	protected.Get("/routes", routeHandler.ListByTenant)
	protected.Get("/routes/:id", routeHandler.GetByID)
	protected.Put("/routes/:id", rbacMiddleware.RequireRole(domain.MemberRoleAdmin), routeHandler.Update)
	protected.Delete("/routes/:id", rbacMiddleware.RequireRole(domain.MemberRoleAdmin), routeHandler.Delete)

	// DLR callbacks (no JWT - called by external providers)
	v1.Post("/dlr/:connector_id", dlrHandler.ReceiveDLR)

	// Audit logs
	protected.Get("/audit-logs", auditHandler.ListByTenant)
	protected.Get("/audit-logs/me", auditHandler.ListByUser)

	slog.Info("routes registered",
		"health", "/health, /ready, /metrics",
		"auth", "/api/v1/auth/*",
		"protected", "/api/v1/tenants, /api/v1/api-keys, /api/v1/audit-logs",
	)

	return app, nil
}

// joinStrings joins a slice of strings with comma.
func joinStrings(s []string) string {
	result := ""
	for i, v := range s {
		if i > 0 {
			result += ", "
		}
		result += v
	}
	return result
}
