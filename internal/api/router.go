package api

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/dreamreflex/service-edge/internal/api/handler"
	"github.com/dreamreflex/service-edge/internal/api/middleware"
	"github.com/dreamreflex/service-edge/internal/config"
)

// Options configures the HTTP router.
type Options struct {
	Handler    *handler.Handler
	JWT        *middleware.JWTManager
	Cfg        *config.Config
	StaticFS   fs.FS // built frontend (may be nil)
	AgentDist  string // dir holding agent binaries served under /download (may be "")
}

// NewRouter builds the gin engine with all routes wired.
func NewRouter(o Options) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.CORS(o.Cfg.CORS.AllowedOrigins))

	r.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	// Public install scripts (token in query).
	r.GET("/install/frps.sh", o.Handler.InstallScript("frps"))
	r.GET("/install/frpc.sh", o.Handler.InstallScript("frpc"))

	// Agent binary downloads.
	if o.AgentDist != "" {
		r.Static("/download", o.AgentDist)
	}

	api := r.Group("/api/v1")

	// Auth (login public; rest protected).
	api.POST("/auth/login", o.Handler.Login)
	authed := api.Group("")
	authed.Use(o.JWT.RequireUser())
	{
		authed.POST("/auth/logout", o.Handler.Logout)
		authed.GET("/auth/me", o.Handler.Me)

		authed.GET("/frps", o.Handler.ListFRPS)
		authed.POST("/frps", o.Handler.CreateFRPS)
		authed.GET("/frps/:uuid", o.Handler.GetFRPS)
		authed.PUT("/frps/:uuid", o.Handler.UpdateFRPS)
		authed.DELETE("/frps/:uuid", o.Handler.DeleteFRPS)
		authed.POST("/frps/:uuid/install-command", o.Handler.InstallCommandFRPS)
		authed.GET("/frps/:uuid/status", o.Handler.FRPSStatus)
		authed.GET("/frps/:uuid/available-ports", o.Handler.AvailablePorts)
		authed.GET("/frps/:uuid/port-usage", o.Handler.PortUsage)

		authed.GET("/frpc", o.Handler.ListFRPC)
		authed.POST("/frpc", o.Handler.CreateFRPC)
		authed.GET("/frpc/:uuid", o.Handler.GetFRPC)
		authed.PUT("/frpc/:uuid", o.Handler.UpdateFRPC)
		authed.DELETE("/frpc/:uuid", o.Handler.DeleteFRPC)
		authed.POST("/frpc/:uuid/install-command", o.Handler.InstallCommandFRPC)
		authed.GET("/frpc/:uuid/status", o.Handler.FRPCStatus)
		authed.GET("/frpc/:uuid/proxies", o.Handler.ListProxies)
		authed.POST("/frpc/:uuid/proxies", o.Handler.AddProxy)

		authed.PUT("/proxies/:id", o.Handler.UpdateProxy)
		authed.DELETE("/proxies/:id", o.Handler.DeleteProxy)

		authed.GET("/ca", o.Handler.CAInfo)
		authed.GET("/topology", o.Handler.Topology)

		authed.GET("/audit-logs", o.Handler.ListAuditLogs)
	}

	// Agent API.
	agentToken := o.Cfg.AgentAPIToken
	agentGrp := api.Group("/agent")
	{
		agentGrp.POST("/enroll", middleware.RequireAgentToken(agentToken), o.Handler.AgentEnroll)

		authedAgent := agentGrp.Group("")
		authedAgent.Use(middleware.RequireAgent(agentToken))
		authedAgent.POST("/heartbeat", o.Handler.AgentHeartbeat)
		authedAgent.POST("/status", o.Handler.AgentStatus)
		authedAgent.GET("/config", o.Handler.AgentConfig)
		authedAgent.POST("/config/ack", o.Handler.AgentConfigAck)
	}

	// Static frontend (SPA fallback).
	if o.StaticFS != nil {
		serveSPA(r, o.StaticFS)
	}

	return r
}

// serveSPA serves built frontend assets and falls back to index.html for client
// routes (anything not under /api, /install, /download).
func serveSPA(r *gin.Engine, static fs.FS) {
	fileServer := http.FileServer(http.FS(static))
	index, _ := fs.ReadFile(static, "index.html")

	r.NoRoute(func(c *gin.Context) {
		p := strings.TrimPrefix(c.Request.URL.Path, "/")
		if p == "" {
			c.Data(http.StatusOK, "text/html; charset=utf-8", index)
			return
		}
		if _, err := fs.Stat(static, p); err == nil {
			fileServer.ServeHTTP(c.Writer, c.Request)
			return
		}
		// Unknown non-API path -> SPA entry point.
		c.Data(http.StatusOK, "text/html; charset=utf-8", index)
	})
}
