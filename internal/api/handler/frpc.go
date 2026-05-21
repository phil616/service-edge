package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/dreamreflex/service-edge/internal/service"
)

// ---- frpc hosts ----

func (h *Handler) ListFRPCHosts(c *gin.Context) {
	hosts, err := h.Svc.ListFRPCHosts()
	if err != nil {
		respondErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": hosts})
}

func (h *Handler) GetFRPCHost(c *gin.Context) {
	host, err := h.Svc.GetFRPCHost(c.Param("uuid"))
	if err != nil {
		respondErr(c, err)
		return
	}
	c.JSON(http.StatusOK, host)
}

func (h *Handler) CreateFRPCHost(c *gin.Context) {
	var in service.CreateFRPCHostInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	host, err := h.Svc.CreateFRPCHost(in)
	if err != nil {
		respondErr(c, err)
		return
	}
	h.auditUser(c, "create_frpc_host", "frpc_host", host.UUID, host.Name)
	c.JSON(http.StatusCreated, host)
}

func (h *Handler) UpdateFRPCHost(c *gin.Context) {
	var in service.UpdateFRPCHostInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	host, err := h.Svc.UpdateFRPCHost(c.Param("uuid"), in)
	if err != nil {
		respondErr(c, err)
		return
	}
	h.auditUser(c, "update_frpc_host", "frpc_host", host.UUID, host.Name)
	c.JSON(http.StatusOK, host)
}

func (h *Handler) DeleteFRPCHost(c *gin.Context) {
	uuid := c.Param("uuid")
	if err := h.Svc.DeleteFRPCHost(uuid); err != nil {
		respondErr(c, err)
		return
	}
	h.auditUser(c, "delete_frpc_host", "frpc_host", uuid, "")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) InstallCommandFRPCHost(c *gin.Context) {
	h.installCommand(c, "frpc")
}

// ---- frpc connections ----

func (h *Handler) ListConnections(c *gin.Context) {
	conns, err := h.Svc.ListConnectionsOfHost(c.Param("uuid"))
	if err != nil {
		respondErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": conns})
}

func (h *Handler) CreateConnection(c *gin.Context) {
	var in service.CreateConnectionInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	conn, err := h.Svc.CreateConnection(c.Param("uuid"), in)
	if err != nil {
		respondErr(c, err)
		return
	}
	h.auditUser(c, "create_connection", "frpc_conn", conn.UUID, conn.Name)
	c.JSON(http.StatusCreated, conn)
}

func (h *Handler) GetConnection(c *gin.Context) {
	conn, err := h.Svc.GetConnection(c.Param("uuid"))
	if err != nil {
		respondErr(c, err)
		return
	}
	conn.TLSCertInfo = h.Svc.LeafCertInfo(conn.TLSCert)
	c.JSON(http.StatusOK, conn)
}

func (h *Handler) UpdateConnection(c *gin.Context) {
	var in service.UpdateConnectionInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	conn, err := h.Svc.UpdateConnection(c.Param("uuid"), in)
	if err != nil {
		respondErr(c, err)
		return
	}
	h.auditUser(c, "update_connection", "frpc_conn", conn.UUID, conn.Name)
	c.JSON(http.StatusOK, conn)
}

func (h *Handler) DeleteConnection(c *gin.Context) {
	uuid := c.Param("uuid")
	if err := h.Svc.DeleteConnection(uuid); err != nil {
		respondErr(c, err)
		return
	}
	h.auditUser(c, "delete_connection", "frpc_conn", uuid, "")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ---- proxies (belong to a connection) ----

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
	h.auditUser(c, "create_proxy", "frpc_conn", uuid, in.Name)
	c.JSON(http.StatusCreated, row)
}

func (h *Handler) UpdateProxy(c *gin.Context) {
	// Parse to 32 bits so the uint() conversion below can never truncate (Go's
	// uint is >=32 bits); proxy ids are GORM auto-increment keys far below 2^32.
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
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
	h.auditUser(c, "update_proxy", "frpc_conn", row.FRPCUUID, in.Name)
	c.JSON(http.StatusOK, row)
}

func (h *Handler) DeleteProxy(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
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
