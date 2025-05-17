package ctx

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/inference/pkg/utils/ctx/errors"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/sentry"
)

type errorResponse struct {
	Message string           `json:"message"`
	SubCode errors.ErrorCode `json:"subCode"`
}

func Success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, data)
}

func SuccessNoContent(c *gin.Context) {
	c.JSON(http.StatusNoContent, nil)
}

func Error(c *gin.Context, err error, msgs ...string) {
	sentry.SendMessage(c, err.Error())

	if errD, ok := err.(errors.InferenceError); ok {
		msg := errD.Message()
		if len(msgs) > 0 {
			msg = fmt.Sprintf("%s, detail: %s", msg, strings.Join(msgs, ", "))
		}
		c.JSON(errD.HTTPCode(), errorResponse{
			Message: msg,
			SubCode: errD.ErrorCode(),
		})
	} else {
		c.JSON(http.StatusInternalServerError, errorResponse{
			Message: "internal server error",
			SubCode: errors.CodeServerInternalError,
		})
	}
}
