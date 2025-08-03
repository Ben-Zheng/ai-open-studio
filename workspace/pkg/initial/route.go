package initial

import (
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/authlib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/sentry"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/workspace/middleware"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/workspace/pkg/api/v1/sla"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/workspace/pkg/api/v1/workspace"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/workspace/pkg/consts"
)

// InitRouter 初始化 gin http 路由
func InitRouter(conf *ginlib.GinConf) {
	consts.GinEngine = ginlib.NewGin(conf)
	consts.GinEngine.Use(sentry.SetGinCollect(), sentry.SetHubScopeTag(sentry.SeverScopeTag, string(types.AIServiceTypeWorkspace)))
	consts.Authenticator = authlib.NewAuthenticator(consts.ConfigMap.AuthConfig)

	consts.GinEngine.GET("/healthz", sla.Healthz)
	rgv1 := consts.GinEngine.Group("/api/v1").Use(
		middleware.ParseQueryParams,
		middleware.ParseURLParams,
		middleware.ParseHearder,
		consts.Authenticator.GinAuthMiddleware(),
	)

	rgv1.GET("/workspaces", workspace.List)
	rgv1.POST("/workspaces", workspace.Create)
	rgv1.GET("/workspaces/:workspaceID", workspace.Detail)
	rgv1.PUT("/workspaces/:workspaceID", workspace.Update)
	rgv1.DELETE("/workspaces/:workspaceID", workspace.Delete)

	rgv1.DELETE("/internal/workspaces/release-by-user", workspace.ReleaseByUser)

	rgv1.GET("/workspaces/:workspaceID/instances", workspace.ListInstance)
	rgv1.POST("/workspaces/:workspaceID/instances", workspace.Startup)
	rgv1.DELETE("/workspaces/:workspaceID/instances/:instanceID", workspace.Shutdown)

	rgv1Toke := consts.GinEngine.Group("/api/v1").Use(
		middleware.ParseQueryParams,
		middleware.ParseURLParams,
		middleware.ParseWorkspaceToken,
	)
	rgv1Toke.GET("/lab/workspaces/:workspaceID/settings", workspace.GetSettings)
	rgv1Toke.POST("/lab/workspaces/:workspaceID/settings", workspace.UpdateSettings)
	rgv1Toke.GET("/lab/workspaces/:workspaceID/instances/:instanceID", workspace.GetInstance)

	rgv1WeakAuth := consts.GinEngine.Group("/api/v1")
	rgv1WeakAuth.Any("/lab/workspaces/:workspaceID/instances/:instanceID/*path", workspace.Proxy)
	rgv1WeakAuth.Any("/vscode/workspaces/:workspaceID/instances/:instanceID/*path", workspace.VSProxy)
}
