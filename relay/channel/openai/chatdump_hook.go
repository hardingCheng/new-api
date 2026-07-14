package openai

import (
	"github.com/QuantumNous/new-api/service/chatdump"

	"github.com/gin-gonic/gin"
)

func getDumpSession(c *gin.Context) *chatdump.Session {
	if v, ok := c.Get("chatdump_session"); ok {
		if s, ok := v.(*chatdump.Session); ok {
			return s
		}
	}
	return nil
}
