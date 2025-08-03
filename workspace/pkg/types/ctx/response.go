package ctx

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/sentry"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/workspace/pkg/types/ctx/errors"
)

type errorResponse struct {
	Message string           `json:"message"`
	SubCode errors.ErrorCode `json:"subCode"`
}

type successResponse struct {
	Data any `json:"data"`
}

func SuccessWithList(c *gin.Context, data any) {
	c.JSON(http.StatusOK, data)
}

func Success(c *gin.Context, data any) {
	c.JSON(http.StatusOK, successResponse{
		Data: data,
	})
}

func SuccessNoContent(c *gin.Context) {
	c.JSON(http.StatusNoContent, nil)
}

func Error(c *gin.Context, err error, msgs ...string) {
	sentry.SendMessage(c, err.Error())

	if errD, ok := err.(errors.WorkspaceServiceError); ok {
		msg := errD.Message()
		if len(msgs) > 0 {
			msg = fmt.Sprintf("%s, detail: %s", msg, strings.Join(msgs, ", "))
		}
		c.AbortWithStatusJSON(errD.HTTPCode(), errorResponse{
			Message: msg,
			SubCode: errD.ErrorCode(),
		})
	} else {
		c.AbortWithStatusJSON(http.StatusInternalServerError, nil)
	}
}

func SuccessWithImage(c *gin.Context, data []byte) {
	c.Data(http.StatusOK, http.DetectContentType(data), data)
}
