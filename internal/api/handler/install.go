package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// InstallScript serves the rendered install script (public, token-gated by query).
func (h *Handler) InstallScript(targetType string) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.Query("token")
		if token == "" {
			c.String(http.StatusBadRequest, "# missing token")
			return
		}
		script, err := h.Svc.RenderInstallScript(targetType, token)
		if err != nil {
			c.String(http.StatusForbidden, "# %s", err.Error())
			return
		}
		c.Header("Content-Type", "text/x-shellscript; charset=utf-8")
		c.String(http.StatusOK, script)
	}
}
