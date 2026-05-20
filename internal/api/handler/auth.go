package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/dreamreflex/service-edge/internal/api/middleware"
)

type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (h *Handler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	user, err := h.Svc.Login(req.Username, req.Password)
	if err != nil {
		h.Svc.Store.Audit(nil, "login_failed", "user", req.Username, "", c.ClientIP())
		respondErr(c, err)
		return
	}
	token, err := h.JWT.Issue(user.ID, user.Username)
	if err != nil {
		respondErr(c, err)
		return
	}
	uid := user.ID
	h.Svc.Store.Audit(&uid, "login", "user", user.Username, "", c.ClientIP())
	c.JSON(http.StatusOK, gin.H{
		"token": token,
		"user":  gin.H{"id": user.ID, "username": user.Username},
	})
}

func (h *Handler) Logout(c *gin.Context) {
	// Stateless JWT: client just drops the token. Recorded for audit.
	h.auditUser(c, "logout", "user", middleware.Username(c), "")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) Me(c *gin.Context) {
	user, err := h.Svc.GetUser(middleware.UserID(c))
	if err != nil {
		respondErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": user.ID, "username": user.Username, "created_at": user.CreatedAt})
}
