package ctx

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/ctx/errors"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/sentry"
)

type errorResponse struct {
	Msg  string           `json:"msg"`
	Code errors.ErrorCode `json:"code"`
	Data any              `json:"data"`
}

func SuccessWithList(c *gin.Context, data any) {
	c.JSON(http.StatusOK, data)
}

func Success(c *gin.Context, data any) {
	c.JSON(http.StatusOK, data)
}

func SuccessNoContent(c *gin.Context) {
	c.JSON(http.StatusNoContent, nil)
}

func Error(c *gin.Context, err error) {
	ErrorWithData(c, err, nil)
}

func ErrorWithData(c *gin.Context, err error, data any) {
	sentry.SendMessage(c, err.Error())

	if errD, ok := err.(*errors.AutoLearnError); ok {
		c.JSON(errD.HTTPCode(), errorResponse{
			Msg:  errD.Message(),
			Code: errD.ErrorCode(),
			Data: data,
		})
	} else {
		c.JSON(http.StatusInternalServerError, nil)
	}
}

func SuccessWithFile(c *gin.Context, filepath, filename string) {
	c.FileAttachment(filepath, filename)
}

func SuccessWithByte(c *gin.Context, filename string, content []byte) {
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Header("Access-Control-Expose-Headers", "Content-Disposition")
	c.Header("Accept-Length", fmt.Sprintf("%d", len(content)))
	c.Data(http.StatusOK, "application/text; charset=utf-8", content)
}
