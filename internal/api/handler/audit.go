package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/dreamreflex/service-edge/internal/model"
)

func (h *Handler) ListAuditLogs(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	q := h.Svc.Store.DB.Model(&model.AuditLog{})
	if action := c.Query("action"); action != "" {
		q = q.Where("action = ?", action)
	}
	if tt := c.Query("target_type"); tt != "" {
		q = q.Where("target_type = ?", tt)
	}

	var total int64
	q.Count(&total)

	var logs []model.AuditLog
	if err := q.Order("id desc").Limit(limit).Offset(offset).Find(&logs).Error; err != nil {
		respondErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": logs, "total": total, "limit": limit, "offset": offset})
}
