package middleware

import (
	"crypto/subtle"
	"net/http"

	"github.com/gin-gonic/gin"
)

const (
	ctxAgentUUID = "agent_uuid"
	ctxAgentType = "agent_type"
)

// RequireAgent validates the shared agent API token and extracts agent identity
// headers. The enroll endpoint uses RequireAgentToken instead (no UUID yet
// registered, identity comes from the enrollment token).
func RequireAgent(agentToken string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !constantEqual(c.GetHeader("X-Agent-Token"), agentToken) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid agent token"})
			return
		}
		uuid := c.GetHeader("X-Agent-UUID")
		atype := c.GetHeader("X-Agent-Type")
		if uuid == "" || (atype != "frps" && atype != "frpc") {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "missing X-Agent-UUID/X-Agent-Type"})
			return
		}
		c.Set(ctxAgentUUID, uuid)
		c.Set(ctxAgentType, atype)
		c.Next()
	}
}

// RequireAgentToken only checks the shared token (used by enroll, where the
// agent isn't registered yet).
func RequireAgentToken(agentToken string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !constantEqual(c.GetHeader("X-Agent-Token"), agentToken) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid agent token"})
			return
		}
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

func constantEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
