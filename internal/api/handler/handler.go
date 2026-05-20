package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/dreamreflex/service-edge/internal/api/middleware"
	"github.com/dreamreflex/service-edge/internal/service"
)

// Handler bundles dependencies shared by all HTTP handlers.
type Handler struct {
	Svc *service.Service
	JWT *middleware.JWTManager
}

func New(svc *service.Service, jwt *middleware.JWTManager) *Handler {
	return &Handler{Svc: svc, JWT: jwt}
}

// respondErr maps service errors to HTTP status codes.
func respondErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrConflict):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrEnrollmentInvalid):
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrInvalidCredentials):
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}

// auditUser writes an audit entry attributed to the current user.
func (h *Handler) auditUser(c *gin.Context, action, targetType, targetUUID, detail string) {
	uid := middleware.UserID(c)
	var uidPtr *uint
	if uid != 0 {
		uidPtr = &uid
	}
	h.Svc.Store.Audit(uidPtr, action, targetType, targetUUID, detail, c.ClientIP())
}
