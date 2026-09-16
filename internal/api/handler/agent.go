package handler

import (
	"crypto/hmac"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/dreamreflex/service-edge/internal/api/middleware"
	"github.com/dreamreflex/service-edge/internal/protocol"
)

// Leave headroom for common 30s reverse-proxy response deadlines.
const longPollTimeout = 15 * time.Second

func (h *Handler) AgentHeartbeat(c *gin.Context) {
	var req protocol.HeartbeatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	uuid := middleware.AgentUUID(c)
	atype := middleware.AgentType(c)
	if row, err := h.Svc.AgentRetirement(atype, uuid); err != nil {
		respondErr(c, err)
		return
	} else if row != nil {
		c.JSON(http.StatusOK, gin.H{"ok": true})
		return
	}
	if err := h.Svc.RecordHeartbeat(atype, uuid, req.ProcessAlive); err != nil {
		respondErr(c, err)
		return
	}
	if err := h.Svc.RecordAppliedVersion(atype, uuid, req.ConfigVersion); err != nil {
		respondErr(c, err)
		return
	}
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
	if row, err := h.Svc.AgentRetirement(atype, uuid); err != nil {
		respondErr(c, err)
		return
	} else if row != nil {
		c.JSON(http.StatusOK, gin.H{"ok": true})
		return
	}
	if err := h.Svc.RecordStatus(atype, uuid, req); err != nil {
		respondErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// AgentConfig is the long-poll endpoint. It hangs up to 15s waiting for a config
// different from current_version; returns 200 + bundle on update, 304 on timeout.
func (h *Handler) AgentConfig(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	uuid := middleware.AgentUUID(c)
	atype := middleware.AgentType(c)
	osName := c.Query("os")
	arch := c.Query("arch")
	currentVersion, parseErr := strconv.Atoi(c.DefaultQuery("current_version", "0"))
	if parseErr != nil || currentVersion < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "current_version must be a non-negative integer"})
		return
	}

	// Retirement bypasses certificate renewal and binary availability entirely.
	retirement, err := h.Svc.AgentRetirement(atype, uuid)
	if err != nil {
		respondErr(c, err)
		return
	}
	if retirement != nil {
		c.JSON(http.StatusOK, gin.H{"config_version": retirement.ConfigVersion, "decommission": true, "connections": []any{}})
		return
	}
	// Renew cert if near expiry (may bump the target version).
	if err := h.Svc.MaybeRenewCert(atype, uuid); err != nil {
		respondErr(c, err)
		return
	}

	deliver := func() bool {
		retirement, err := h.Svc.AgentRetirement(atype, uuid)
		if err != nil {
			respondErr(c, err)
			return true
		}
		if retirement != nil {
			c.JSON(http.StatusOK, gin.H{"config_version": retirement.ConfigVersion, "decommission": true, "connections": []any{}})
			return true
		}
		target, err := h.Svc.CurrentConfigVersion(atype, uuid)
		if err != nil {
			respondErr(c, err)
			return true
		}
		if target != currentVersion {
			var bundle any
			var err error
			if atype == "frpc" {
				// frpc agents are hosts: deliver the full set of connections.
				bundle, err = h.Svc.BuildHostConfig(uuid, osName, arch)
			} else {
				bundle, err = h.Svc.BuildConfigResponse(atype, uuid, osName, arch)
			}
			if err != nil {
				respondErr(c, err)
				return true
			}
			c.JSON(http.StatusOK, bundle)
			return true
		}
		return false
	}

	ch, unsub := h.Svc.Notifier.Subscribe(uuid)
	defer unsub()
	if deliver() {
		return
	}

	timer := time.NewTimer(longPollTimeout)
	defer timer.Stop()

	select {
	case <-ch:
		if deliver() {
			return
		}
		c.Status(http.StatusNotModified)
	case <-timer.C:
		if deliver() {
			return
		}
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
	if err := h.Svc.RecordConfigAck(atype, uuid, req); err != nil {
		respondErr(c, err)
		return
	}
	if req.Success {
		h.Svc.Store.Audit(nil, "config_applied", atype, uuid, "version="+strconv.Itoa(req.ConfigVersion), c.ClientIP())
	} else {
		h.Svc.Store.Audit(nil, "config_apply_failed", atype, uuid, req.Error, c.ClientIP())
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// AgentEnroll consumes a one-time enrollment token (token must match the
// uuid/type recorded for it), plus the credential bound to that identity.
func (h *Handler) AgentEnroll(c *gin.Context) {
	token := c.Query("token")
	var req protocol.EnrollRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !hmac.Equal([]byte(c.GetHeader("X-Agent-Token")), []byte(protocol.AgentToken(h.Svc.Cfg.AgentAPIToken, req.AgentType, req.UUID))) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid enrollment identity"})
		return
	}
	if _, err := h.Svc.ConsumeEnrollment(token, req.UUID, req.AgentType); err != nil {
		respondErr(c, err)
		return
	}
	h.Svc.Store.Audit(nil, "agent_enrolled", req.AgentType, req.UUID, "", c.ClientIP())
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
