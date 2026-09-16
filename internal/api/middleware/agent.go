package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

const (
	ctxAgentUUID = "agent_uuid"
	ctxAgentType = "agent_type"
)

// RequireAgent authenticates the role/UUID together and checks enrollment or
// a retained retirement record. Unknown identities never reach config handlers.
func RequireAgent(authorize func(kind, uuid, token string) (bool, error)) gin.HandlerFunc {
	return func(c *gin.Context) {
		uuid, kind := c.GetHeader("X-Agent-UUID"), c.GetHeader("X-Agent-Type")
		if uuid == "" || (kind != "frps" && kind != "frpc") {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "missing X-Agent-UUID/X-Agent-Type"})
			return
		}
		ok, err := authorize(kind, uuid, c.GetHeader("X-Agent-Token"))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "agent authentication unavailable"})
			return
		}
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or unenrolled agent identity"})
			return
		}
		c.Set(ctxAgentUUID, uuid)
		c.Set(ctxAgentType, kind)
		c.Next()
	}
}

func AgentUUID(c *gin.Context) string {
	if v, ok := c.Get(ctxAgentUUID); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func AgentType(c *gin.Context) string {
	if v, ok := c.Get(ctxAgentType); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
