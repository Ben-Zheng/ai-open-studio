package sla

import (
	"github.com/gin-gonic/gin"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/workspace/pkg/types/ctx"
)

func Healthz(c *gin.Context) {
	ctx.SuccessNoContent(c)
}
