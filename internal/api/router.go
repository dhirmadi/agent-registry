package api

import (
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/agent-smit/agentic-registry/internal/auth"
	"github.com/agent-smit/agentic-registry/internal/ratelimit"
)

// AuthRouteHandler defines the methods the router needs from the auth handler.
type AuthRouteHandler interface {
	HandleLogin(w http.ResponseWriter, r *http.Request)
	HandleLogout(w http.ResponseWriter, r *http.Request)
	HandleMe(w http.ResponseWriter, r *http.Request)
	HandleChangePassword(w http.ResponseWriter, r *http.Request)
	HandleGoogleStart(w http.ResponseWriter, r *http.Request)
	HandleGoogleCallback(w http.ResponseWriter, r *http.Request)
	HandleUnlinkGoogle(w http.ResponseWriter, r *http.Request)
}

// RouterConfig holds all dependencies needed to build the router.
type RouterConfig struct {
	Health        *HealthHandler
	Auth          AuthRouteHandler
	Users         *UsersHandler
	APIKeys       *APIKeysHandler
	Agents        *AgentsHandler
	Prompts       *PromptsHandler
	MCPServers    *MCPServersHandler
	TrustRules    *TrustRulesHandler
	TrustDefaults *TrustDefaultsHandler
	ModelConfig    *ModelConfigHandler
	ModelEndpoints *ModelEndpointsHandler
	Webhooks      *WebhooksHandler
	Discovery     *DiscoveryHandler
	AuditLog      *AuditHandler
	AuthMW        func(http.Handler) http.Handler
	UserLookup    UserLookup             // For MustChangePassMiddleware (nil = no enforcement)
	RateLimiter   *ratelimit.RateLimiter // nil = no rate limiting
	WebFS         fs.FS                  // Embedded SPA filesystem (nil = no SPA serving)
}

// NewRouter creates the chi router with middleware and all routes.
func NewRouter(cfg RouterConfig) chi.Router {
	r := chi.NewRouter()

	// Standard middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(CORSMiddleware)
	r.Use(securityHeaders)

	// Health routes (no auth required)
	r.Get("/healthz", cfg.Health.Healthz)
	r.Get("/readyz", cfg.Health.Readyz)

	// Auth routes
	if cfg.Auth != nil {
		r.Route("/auth", func(r chi.Router) {
			// Public auth routes (no auth required)
			// Rate limit login: 5 requests per 15 minutes per IP (brute-force protection)
			if cfg.RateLimiter != nil {
				r.With(cfg.RateLimiter.Middleware(5, 15*time.Minute, func(r *http.Request) string {
					return "login:" + r.RemoteAddr
				})).Post("/login", cfg.Auth.HandleLogin)
			} else {
				r.Post("/login", cfg.Auth.HandleLogin)
			}
			// Rate limit Google OAuth: 10 requests per 15 minutes per IP
			if cfg.RateLimiter != nil {
				r.With(cfg.RateLimiter.Middleware(10, 15*time.Minute, func(r *http.Request) string {
					return "oauth:" + r.RemoteAddr
				})).Get("/google/start", cfg.Auth.HandleGoogleStart)
				r.With(cfg.RateLimiter.Middleware(10, 15*time.Minute, func(r *http.Request) string {
					return "oauth:" + r.RemoteAddr
				})).Get("/google/callback", cfg.Auth.HandleGoogleCallback)
			} else {
				r.Get("/google/start", cfg.Auth.HandleGoogleStart)
				r.Get("/google/callback", cfg.Auth.HandleGoogleCallback)
			}

			// Protected auth routes (auth required)
			r.Group(func(r chi.Router) {
				if cfg.AuthMW != nil {
					r.Use(cfg.AuthMW)
				}
				r.Post("/logout", cfg.Auth.HandleLogout)
				r.Get("/me", cfg.Auth.HandleMe)
				r.Post("/change-password", cfg.Auth.HandleChangePassword)
				r.Post("/unlink-google", cfg.Auth.HandleUnlinkGoogle)
			})
		})
	}

	// API routes (all require auth)
	r.Route("/api/v1", func(r chi.Router) {
		// Limit request body size to 1MB for all API routes
		r.Use(MaxBodySize(1 << 20))

		if cfg.AuthMW != nil {
			r.Use(cfg.AuthMW)
		}

		if cfg.UserLookup != nil {
			r.Use(MustChangePassMiddleware(cfg.UserLookup))
		}

		// Rate limit API: 60/min for mutations (POST/PUT/PATCH/DELETE),
		// 300/min for reads (GET), per authenticated user or API key.
		if cfg.RateLimiter != nil {
			r.Use(methodAwareRateLimiter(cfg.RateLimiter))
		}

		// Users (admin only)
		if cfg.Users != nil {
			r.Route("/users", func(r chi.Router) {
				r.Use(RequireRole("admin"))
				r.Get("/", cfg.Users.List)
				r.Post("/", cfg.Users.Create)
				r.Get("/{userId}", cfg.Users.Get)
				r.Put("/{userId}", cfg.Users.Update)
				r.Delete("/{userId}", cfg.Users.Delete)
				r.Post("/{userId}/reset-auth", cfg.Users.ResetAuth)
			})
		}

		// API Keys (auth required, no specific role)
		if cfg.APIKeys != nil {
			r.Route("/api-keys", func(r chi.Router) {
				r.Get("/", cfg.APIKeys.List)
				r.Post("/", cfg.APIKeys.Create)
				r.Delete("/{keyId}", cfg.APIKeys.Revoke)
			})
		}

		// Agents
		if cfg.Agents != nil {
			r.Route("/agents", func(r chi.Router) {
				// Read endpoints (viewer+)
				r.Group(func(r chi.Router) {
					r.Use(RequireRole("viewer", "editor", "admin"))
					r.Get("/", cfg.Agents.List)
					r.Get("/{agentId}", cfg.Agents.Get)
					r.Get("/{agentId}/versions", cfg.Agents.ListVersions)
					r.Get("/{agentId}/versions/{version}", cfg.Agents.GetVersion)
				})

				// Write endpoints (editor+)
				r.Group(func(r chi.Router) {
					r.Use(RequireRole("editor", "admin"))
					r.Post("/", cfg.Agents.Create)
					r.Put("/{agentId}", cfg.Agents.Update)
					r.Patch("/{agentId}", cfg.Agents.PatchAgent)
					r.Delete("/{agentId}", cfg.Agents.Delete)
					r.Post("/{agentId}/rollback", cfg.Agents.Rollback)
				})

				// Prompts (nested under agents)
				if cfg.Prompts != nil {
					r.Route("/{agentId}/prompts", func(r chi.Router) {
						// Read endpoints (viewer+)
						r.Group(func(r chi.Router) {
							r.Use(RequireRole("viewer", "editor", "admin"))
							r.Get("/", cfg.Prompts.List)
							r.Get("/active", cfg.Prompts.GetActive)
							r.Get("/{promptId}", cfg.Prompts.GetByID)
						})

						// Write endpoints (editor+)
						r.Group(func(r chi.Router) {
							r.Use(RequireRole("editor", "admin"))
							r.Post("/", cfg.Prompts.Create)
							r.Post("/{promptId}/activate", cfg.Prompts.Activate)
							r.Post("/rollback", cfg.Prompts.Rollback)
						})
					})
				}
			})
		}

		// MCP Servers (admin only)
		if cfg.MCPServers != nil {
			r.Route("/mcp-servers", func(r chi.Router) {
				r.Use(RequireRole("admin"))
				r.Get("/", cfg.MCPServers.List)
				r.Post("/", cfg.MCPServers.Create)
				r.Get("/{serverId}", cfg.MCPServers.Get)
				r.Put("/{serverId}", cfg.MCPServers.Update)
				r.Delete("/{serverId}", cfg.MCPServers.Delete)
			})
		}

		// Trust Defaults (admin only)
		if cfg.TrustDefaults != nil {
			r.Route("/trust-defaults", func(r chi.Router) {
				r.Use(RequireRole("admin"))
				r.Get("/", cfg.TrustDefaults.List)
				r.Put("/{defaultId}", cfg.TrustDefaults.Update)
			})
		}

		// Model Config (admin only for global)
		if cfg.ModelConfig != nil {
			r.Route("/model-config", func(r chi.Router) {
				r.Use(RequireRole("admin"))
				r.Get("/", cfg.ModelConfig.GetGlobal)
				r.Put("/", cfg.ModelConfig.UpdateGlobal)
			})
		}

		// Model Endpoints
		if cfg.ModelEndpoints != nil {
			r.Route("/model-endpoints", func(r chi.Router) {
				// Read endpoints (viewer+)
				r.Group(func(r chi.Router) {
					r.Use(RequireRole("viewer", "editor", "admin"))
					r.Get("/", cfg.ModelEndpoints.List)
					r.Get("/{slug}", cfg.ModelEndpoints.Get)
					r.Get("/{slug}/versions", cfg.ModelEndpoints.ListVersions)
					r.Get("/{slug}/versions/{version}", cfg.ModelEndpoints.GetVersion)
				})

				// Write endpoints (editor+)
				r.Group(func(r chi.Router) {
					r.Use(RequireRole("editor", "admin"))
					r.Post("/", cfg.ModelEndpoints.Create)
					r.Put("/{slug}", cfg.ModelEndpoints.Update)
					r.Delete("/{slug}", cfg.ModelEndpoints.Delete)
					r.Post("/{slug}/versions", cfg.ModelEndpoints.CreateVersion)
					r.Post("/{slug}/versions/{version}/activate", cfg.ModelEndpoints.ActivateVersion)
				})
			})
		}

		// Webhooks (admin only)
		if cfg.Webhooks != nil {
			r.Route("/webhooks", func(r chi.Router) {
				r.Use(RequireRole("admin"))
				r.Get("/", cfg.Webhooks.List)
				r.Post("/", cfg.Webhooks.Create)
				r.Delete("/{webhookId}", cfg.Webhooks.Delete)
			})
		}

		// Audit Log (admin only)
		if cfg.AuditLog != nil {
			r.Route("/audit-log", func(r chi.Router) {
				r.Use(RequireRole("admin"))
				r.Get("/", cfg.AuditLog.List)
			})
		}

		// Discovery (viewer+) — rate limited at 10/min per user/key
		if cfg.Discovery != nil {
			r.Group(func(r chi.Router) {
				r.Use(RequireRole("viewer", "editor", "admin"))
				if cfg.RateLimiter != nil {
					r.Use(cfg.RateLimiter.Middleware(10, time.Minute, func(r *http.Request) string {
						if uid, ok := auth.UserIDFromContext(r.Context()); ok {
							return "discovery:" + uid.String()
						}
						return "discovery:" + r.RemoteAddr
					}))
				}
				r.Get("/discovery", cfg.Discovery.GetDiscovery)
			})
		}

		// Workspace-scoped routes
		r.Route("/workspaces/{workspaceId}", func(r chi.Router) {
			// Trust Rules (editor+)
			if cfg.TrustRules != nil {
				r.Route("/trust-rules", func(r chi.Router) {
					r.Use(RequireRole("editor", "admin"))
					r.Get("/", cfg.TrustRules.List)
					r.Post("/", cfg.TrustRules.Create)
					r.Delete("/{ruleId}", cfg.TrustRules.Delete)
				})
			}

			// Model Config (workspace-scoped, editor+)
			if cfg.ModelConfig != nil {
				r.Route("/model-config", func(r chi.Router) {
					r.Use(RequireRole("editor", "admin"))
					r.Get("/", cfg.ModelConfig.GetWorkspace)
					r.Put("/", cfg.ModelConfig.UpdateWorkspace)
				})
			}

			// Model Endpoints (workspace-scoped)
			if cfg.ModelEndpoints != nil {
				r.Route("/model-endpoints", func(r chi.Router) {
					r.Group(func(r chi.Router) {
						r.Use(RequireRole("viewer", "editor", "admin"))
						r.Get("/", cfg.ModelEndpoints.ListByWorkspace)
					})
					r.Group(func(r chi.Router) {
						r.Use(RequireRole("editor", "admin"))
						r.Post("/", cfg.ModelEndpoints.CreateForWorkspace)
					})
				})
			}
		})
	})

	// Serve embedded SPA — catch-all after all API/auth/health routes
	if cfg.WebFS != nil {
		spaFS, err := fs.Sub(cfg.WebFS, "dist")
		if err == nil {
			handler := spaHandler(spaFS)
			r.Get("/", handler)
			r.Get("/*", handler)
		}
	}

	return r
}

// methodAwareRateLimiter applies different rate limits for read vs mutation HTTP methods.
// Mutations (POST/PUT/PATCH/DELETE): 60 requests/min per authenticated user.
// Reads (GET and others): 300 requests/min per authenticated user.
func methodAwareRateLimiter(rl *ratelimit.RateLimiter) func(http.Handler) http.Handler {
	mutationMW := rl.Middleware(60, time.Minute, func(r *http.Request) string {
		if uid, ok := auth.UserIDFromContext(r.Context()); ok {
			return "api-mutation:" + uid.String()
		}
		return "api-mutation:" + r.RemoteAddr
	})
	readMW := rl.Middleware(300, time.Minute, func(r *http.Request) string {
		if uid, ok := auth.UserIDFromContext(r.Context()); ok {
			return "api-read:" + uid.String()
		}
		return "api-read:" + r.RemoteAddr
	})

	return func(next http.Handler) http.Handler {
		mutationHandler := mutationMW(next)
		readHandler := readMW(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
				mutationHandler.ServeHTTP(w, r)
			default:
				readHandler.ServeHTTP(w, r)
			}
		})
	}
}

// spaHandler serves static files from the embedded filesystem, falling back
// to index.html for client-side routes (SPA routing).
func spaHandler(fsys fs.FS) http.HandlerFunc {
	fileServer := http.FileServer(http.FS(fsys))

	return func(w http.ResponseWriter, r *http.Request) {
		// Clean the path and strip leading slash for fs.Open
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")

		// Try to open the requested file
		f, err := fsys.Open(p)
		if err != nil {
			// File not found — serve index.html for SPA routing
			serveIndexHTML(w, r, fsys)
			return
		}
		f.Close()

		// If it's a directory, serve index.html instead
		info, err := fs.Stat(fsys, p)
		if err != nil || info.IsDir() {
			serveIndexHTML(w, r, fsys)
			return
		}

		// Serve the static file
		fileServer.ServeHTTP(w, r)
	}
}

// serveIndexHTML reads and serves the index.html from the embedded FS.
func serveIndexHTML(w http.ResponseWriter, _ *http.Request, fsys fs.FS) {
	f, err := fsys.Open("index.html")
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	defer f.Close()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	io.Copy(w, f)
}

// securityHeaders adds security-related HTTP headers to every response.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		next.ServeHTTP(w, r)
	})
}
