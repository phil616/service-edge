package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/dreamreflex/service-edge/internal/api/middleware"
	"github.com/dreamreflex/service-edge/internal/protocol"
)

const longPollTimeout = 30 * time.Second

func (h *Handler) AgentHeartbeat(c *gin.Context) {
	var req protocol.HeartbeatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	uuid := middleware.AgentUUID(c)
	atype := middleware.AgentType(c)
	if err := h.Svc.RecordHeartbeat(atype, uuid, req.ProcessAlive); err != nil {
		respondErr(c, err)
		return
	}
	// Learn the frps public IP from the address it connects from (if unset).
	h.Svc.NoteFRPSPublicIP(atype, uuid, c.ClientIP())
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) AgentStatus(c *gin.Context) {
	var req protocol.StatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	uuid := middleware.AgentUUID(c)
	atype := middleware.AgentType(c)
	if err := h.Svc.RecordStatus(atype, uuid, req); err != nil {
		respondErr(c, err)
		return
	}
	h.Svc.NoteFRPSPublicIP(atype, uuid, c.ClientIP())
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// AgentConfig is the long-poll endpoint. It hangs up to 30s waiting for a config
// newer than current_version; returns 200 + bundle on update, 304 on timeout.
func (h *Handler) AgentConfig(c *gin.Context) {
	uuid := middleware.AgentUUID(c)
	atype := middleware.AgentType(c)
	osName := c.Query("os")
	arch := c.Query("arch")
	currentVersion, _ := strconv.Atoi(c.Query("current_version"))

	// Renew cert if near expiry (may bump the target version).
	if err := h.Svc.MaybeRenewCert(atype, uuid); err != nil {
		respondErr(c, err)
		return
	}

	deliver := func() bool {
		target, err := h.Svc.CurrentConfigVersion(atype, uuid)
		if err != nil {
			respondErr(c, err)
			return true
		}
		if target > currentVersion {
			bundle, err := h.Svc.BuildConfigResponse(atype, uuid, osName, arch)
			if err != nil {
				respondErr(c, err)
				return true
			}
			c.JSON(http.StatusOK, bundle)
			return true
		}
		return false
	}

	if deliver() {
		return
	}

	ch, unsub := h.Svc.Notifier.Subscribe(uuid)
	defer unsub()

	timer := time.NewTimer(longPollTimeout)
	defer timer.Stop()

	select {
	case <-ch:
		if deliver() {
			return
		}
		c.Status(http.StatusNotModified)
	case <-timer.C:
		c.Status(http.StatusNotModified)
	case <-c.Request.Context().Done():
		c.Status(http.StatusNotModified)
	}
}

func (h *Handler) AgentConfigAck(c *gin.Context) {
	var req protocol.AckRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	uuid := middleware.AgentUUID(c)
	atype := middleware.AgentType(c)
	if req.Success {
		h.Svc.Store.Audit(nil, "config_applied", atype, uuid, "version="+strconv.Itoa(req.ConfigVersion), c.ClientIP())
	} else {
		h.Svc.Store.Audit(nil, "config_apply_failed", atype, uuid, req.Error, c.ClientIP())
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// AgentEnroll consumes a one-time enrollment token (token must match the
// uuid/type recorded for it). Guarded by the shared agent token only.
func (h *Handler) AgentEnroll(c *gin.Context) {
	token := c.Query("token")
	var req protocol.EnrollRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if _, err := h.Svc.ConsumeEnrollment(token, req.UUID, req.AgentType); err != nil {
		respondErr(c, err)
		return
	}
	h.Svc.Store.Audit(nil, "agent_enrolled", req.AgentType, req.UUID, "", c.ClientIP())
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
