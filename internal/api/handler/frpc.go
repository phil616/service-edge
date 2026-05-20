package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/dreamreflex/service-edge/internal/service"
)

func (h *Handler) ListFRPC(c *gin.Context) {
	clients, err := h.Svc.ListFRPC()
	if err != nil {
		respondErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": clients})
}

func (h *Handler) GetFRPC(c *gin.Context) {
	client, err := h.Svc.GetFRPC(c.Param("uuid"))
	if err != nil {
		respondErr(c, err)
		return
	}
	c.JSON(http.StatusOK, client)
}

func (h *Handler) CreateFRPC(c *gin.Context) {
	var in service.CreateFRPCInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	client, err := h.Svc.CreateFRPC(in)
	if err != nil {
		respondErr(c, err)
		return
	}
	h.auditUser(c, "create_frpc", "frpc", client.UUID, client.Name)
	c.JSON(http.StatusCreated, client)
}

func (h *Handler) UpdateFRPC(c *gin.Context) {
	var in service.UpdateFRPCInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	client, err := h.Svc.UpdateFRPC(c.Param("uuid"), in)
	if err != nil {
		respondErr(c, err)
		return
	}
	h.auditUser(c, "update_frpc", "frpc", client.UUID, client.Name)
	c.JSON(http.StatusOK, client)
}

func (h *Handler) DeleteFRPC(c *gin.Context) {
	uuid := c.Param("uuid")
	if err := h.Svc.DeleteFRPC(uuid); err != nil {
		respondErr(c, err)
		return
	}
	h.auditUser(c, "delete_frpc", "frpc", uuid, "")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) FRPCStatus(c *gin.Context) {
	client, err := h.Svc.GetFRPC(c.Param("uuid"))
	if err != nil {
		respondErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"uuid":           client.UUID,
		"status":         client.Status,
		"last_heartbeat": client.LastHeartbeat,
		"config_version": client.ConfigVersion,
		"frp_version":    client.FrpVersion,
		"frps_uuid":      client.FRPSUUID,
		"proxy_count":    len(client.Proxies),
	})
}

func (h *Handler) InstallCommandFRPC(c *gin.Context) {
	h.installCommand(c, "frpc")
}

// ---- proxies ----

func (h *Handler) ListProxies(c *gin.Context) {
	proxies, err := h.Svc.ListProxies(c.Param("uuid"))
	if err != nil {
		respondErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": proxies})
}

func (h *Handler) AddProxy(c *gin.Context) {
	var in service.ProxyMappingInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	uuid := c.Param("uuid")
	row, err := h.Svc.AddProxy(uuid, in)
	if err != nil {
		respondErr(c, err)
		return
	}
	h.auditUser(c, "create_proxy", "frpc", uuid, in.Name)
	c.JSON(http.StatusCreated, row)
}

func (h *Handler) UpdateProxy(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var in service.ProxyMappingInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	row, err := h.Svc.UpdateProxy(uint(id), in)
	if err != nil {
		respondErr(c, err)
		return
	}
	h.auditUser(c, "update_proxy", "frpc", row.FRPCUUID, in.Name)
	c.JSON(http.StatusOK, row)
}

func (h *Handler) DeleteProxy(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.Svc.DeleteProxy(uint(id)); err != nil {
		respondErr(c, err)
		return
	}
	h.auditUser(c, "delete_proxy", "proxy", strconv.FormatUint(id, 10), "")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
