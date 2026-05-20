package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/dreamreflex/service-edge/internal/service"
)

func (h *Handler) ListFRPS(c *gin.Context) {
	nodes, err := h.Svc.ListFRPS()
	if err != nil {
		respondErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": nodes})
}

func (h *Handler) GetFRPS(c *gin.Context) {
	node, err := h.Svc.GetFRPS(c.Param("uuid"))
	if err != nil {
		respondErr(c, err)
		return
	}
	c.JSON(http.StatusOK, node)
}

func (h *Handler) CreateFRPS(c *gin.Context) {
	var in service.CreateFRPSInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	node, err := h.Svc.CreateFRPS(in)
	if err != nil {
		respondErr(c, err)
		return
	}
	h.auditUser(c, "create_frps", "frps", node.UUID, node.Name)
	c.JSON(http.StatusCreated, node)
}

func (h *Handler) UpdateFRPS(c *gin.Context) {
	var in service.UpdateFRPSInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	node, err := h.Svc.UpdateFRPS(c.Param("uuid"), in)
	if err != nil {
		respondErr(c, err)
		return
	}
	h.auditUser(c, "update_frps", "frps", node.UUID, node.Name)
	c.JSON(http.StatusOK, node)
}

func (h *Handler) DeleteFRPS(c *gin.Context) {
	uuid := c.Param("uuid")
	if err := h.Svc.DeleteFRPS(uuid); err != nil {
		respondErr(c, err)
		return
	}
	h.auditUser(c, "delete_frps", "frps", uuid, "")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) FRPSStatus(c *gin.Context) {
	node, err := h.Svc.GetFRPS(c.Param("uuid"))
	if err != nil {
		respondErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"uuid":           node.UUID,
		"status":         node.Status,
		"last_heartbeat": node.LastHeartbeat,
		"config_version": node.ConfigVersion,
		"frp_version":    node.FrpVersion,
		"public_ip":      node.PublicIP,
	})
}

func (h *Handler) AvailablePorts(c *gin.Context) {
	used, err := h.Svc.UsedRemotePorts(c.Param("uuid"))
	if err != nil {
		respondErr(c, err)
		return
	}
	usedList := make([]int, 0, len(used))
	for p := range used {
		usedList = append(usedList, p)
	}
	c.JSON(http.StatusOK, gin.H{"used_ports": usedList})
}

// InstallCommandFRPS generates a one-time install command for an frps node.
func (h *Handler) InstallCommandFRPS(c *gin.Context) {
	h.installCommand(c, "frps")
}

func (h *Handler) installCommand(c *gin.Context, targetType string) {
	uuid := c.Param("uuid")
	// Verify target exists.
	var err error
	if targetType == "frps" {
		_, err = h.Svc.GetFRPS(uuid)
	} else {
		_, err = h.Svc.GetFRPC(uuid)
	}
	if err != nil {
		respondErr(c, err)
		return
	}
	tok, err := h.Svc.CreateEnrollment(targetType, uuid)
	if err != nil {
		respondErr(c, err)
		return
	}
	h.auditUser(c, "generate_install_command", targetType, uuid, "")
	c.JSON(http.StatusOK, gin.H{
		"command":    h.Svc.InstallCommand(targetType, tok.Token),
		"token":      tok.Token,
		"expires_at": tok.ExpiresAt,
	})
}
