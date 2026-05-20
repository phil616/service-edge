package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// CAInfo returns details of the control-plane CA certificate.
func (h *Handler) CAInfo(c *gin.Context) {
	c.JSON(http.StatusOK, h.Svc.CAInfo())
}

// Topology returns the full frps/frpc/proxy graph for the network diagram.
func (h *Handler) Topology(c *gin.Context) {
	topo, err := h.Svc.Topology()
	if err != nil {
		respondErr(c, err)
		return
	}
	c.JSON(http.StatusOK, topo)
}

// PortUsage returns the detailed occupied-port list for an frps node.
func (h *Handler) PortUsage(c *gin.Context) {
	items, err := h.Svc.PortUsage(c.Param("uuid"))
	if err != nil {
		respondErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}
