package handler

import (
	"github.com/dreamreflex/service-edge/internal/model"
	"github.com/gin-gonic/gin"
	"net/http"
)

func (h *Handler) ListAgentRetirements(c *gin.Context) {
	rows := []model.AgentRetirement{}
	if err := h.Svc.Store.DB.Order("created_at desc").Limit(200).Find(&rows).Error; err != nil {
		respondErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rows})
}
