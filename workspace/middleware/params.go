package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	authType "go.megvii-inc.com/brain/brainpp/projects/aiservice/auth/pkg/app/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/workspace/pkg/consts"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/workspace/pkg/types/ctx"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/workspace/pkg/types/ctx/errors"
)

type QueryParams struct {
	PageSize int    `json:"pageSize" form:"pageSize"`
	Page     int    `json:"page" form:"page"`
	SortBy   string `json:"sortBy" form:"sortBy"`
	Order    string `json:"order" form:"order"`
	Name     string `json:"name" form:"name"`
}

func ParseQueryParams(c *gin.Context) {
	var qp = QueryParams{
		PageSize: 10,
		Page:     1,
		Order:    consts.SortByDESC,
		SortBy:   "id",
	}
	if err := c.ShouldBindQuery(&qp); err != nil {
		ctx.Error(c, errors.ErrParamsInvalid)
		c.Abort()
		return
	}

	logrus.Infof("--------ce ce ce-------: %+v", qp)
	c.Set(consts.PageSize, qp.PageSize)
	c.Set(consts.Page, qp.Page)
	c.Set(consts.SortBy, qp.SortBy)
	c.Set(consts.Order, qp.Order)
	c.Set(consts.Name, qp.Name)
}

type URLParams struct {
	WorkspaceID string `uri:"workspaceID" form:"workspaceID"`
	InstanceID  string `uri:"instanceID" form:"instanceID"`
}

func ParseURLParams(c *gin.Context) {
	var up = URLParams{}
	if err := c.ShouldBindUri(&up); err != nil {
		logrus.Error("parse URL params failed")
		ctx.Error(c, errors.ErrParamsInvalid)
		c.Abort()
		return
	}

	logrus.Infof("--------ce ce ce-------: %+v", up)
	c.Set(consts.WorkspaceID, up.WorkspaceID)
	c.Set(consts.InstanceID, up.InstanceID)

	c.Next()
}

func ParseHearder(c *gin.Context) {
	userID := c.GetHeader(authType.AISSubjectHeader)
	if userID == "" {
		logrus.Warn("header user id get failed")
	} else {
		c.Set(consts.UserID, userID)
	}

	tenantID := c.GetHeader(authType.AISTenantHeader)
	if tenantID == "" {
		logrus.Warn("header tenant id get failed")
	} else {
		c.Set(consts.TenantID, tenantID)
	}

	projectID := c.GetHeader(authType.AISProjectHeader)
	if projectID == "" {
		logrus.Warn("header project id get failed")
	} else {
		c.Set(consts.ProjectID, projectID)
	}
}

func ParseWorkspaceToken(c *gin.Context) {
	token := c.GetHeader(consts.HeaderToken)
	if token == "" {
		logrus.Error("header token get failed")
		ctx.Error(c, errors.ErrParamsInvalid)
		c.Abort()

		return
	}
	c.Set(consts.Token, token)
}
