package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// GetSettings returns the agent download settings (control-plane default + overrides).
func (h *Handler) GetSettings(c *gin.Context) {
	c.JSON(http.StatusOK, h.Svc.GetAgentDownloadSettings())
}

type updateSettingsInput struct {
	FRPSAgentDownloadURL string `json:"agent_download_url_frps"`
	FRPCAgentDownloadURL string `json:"agent_download_url_frpc"`
}

// UpdateSettings persists per-type agent download URL overrides (empty clears).
func (h *Handler) UpdateSettings(c *gin.Context) {
	var in updateSettingsInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.Svc.UpdateAgentDownloadSettings(in.FRPSAgentDownloadURL, in.FRPCAgentDownloadURL); err != nil {
		respondErr(c, err)
		return
	}
	h.auditUser(c, "update_settings", "settings", "agent_download", "")
	c.JSON(http.StatusOK, h.Svc.GetAgentDownloadSettings())
}
