package initial

import (
	"github.com/gin-gonic/gin"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/sentry"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/api/v1/algoresource"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/api/v1/quota"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/api/v1/resource"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/api/v1/site"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/consts"
)

// InitRouter 初始化 gin http 路由
func InitRouter(conf *ginlib.GinConf) {
	consts.GinEngine = ginlib.NewGin(conf)
	consts.GinEngine.Use(sentry.SetGinCollect(), sentry.SetHubScopeTag(sentry.SeverScopeTag, string(types.AIServiceTypePublic)))

	rgv1 := consts.GinEngine.Group("/api/v1")

	// 资源管理相关
	InitResourceRouter(rgv1)
	// 配额管理相关
	InitQuotaRouter(rgv1)      //  用户端
	InitQuotaAdminRouter(rgv1) // 运营端

	InitSiteRouter(rgv1) // 多集群相关

	InitAlgoResource(rgv1) // 算法仓分配
}

func InitResourceRouter(rgv1 *gin.RouterGroup) {
	// 资源管理
	rgv1.GET("/audits", resource.ListAudit) // 事件记录（用户端/运营端）
	rgv1.POST("/audits", resource.CreateAudit)

	// 资源监控图
	rgv1.GET("/metrics/query_range", resource.GetMetrics)                  // 机器资源统计和团队配额占用统计（用户端/运营端），这里需要前端区分一下 查询的是配额统计/还是资源呢占用统计
	rgv1.GET("/metrics/pods/:podName/query_range", resource.GetPodMetrics) // 工作空间pod资源监控

	// 机器管理
	rgv1.GET("/nodes", resource.ListNodes)                 // 机器列表
	rgv1.PUT("/nodes/:nodeID", resource.ManageNode)        // 更新机器（是否可调度）
	rgv1.GET("/nodes/:nodeID", resource.NodeDetail)        // 机器详情
	rgv1.GET("/nodes/:nodeID/pods", resource.ListNodePods) // 机器中pod列表

	// 模型转换时查看机器
	rgv1.GET("/nodes/resourcesense", quota.NodesResourceSense) // 机器资源感知

	// 界面上无调用
	rgv1.GET("/node-details", resource.GetDetails) // 查看从kubebrain获取的节点信息
	rgv1.GET("/node-details/:nodeName/pods", resource.GetNodeDetails)

	// 没有开启配额时使用
	rgv1.GET("/resources", resource.ListResource)  // 这两个接口需要替换为新的地址, prometheus使用
	rgv1.GET("/resources/detail", resource.Detail) // 这两个接口需要替换为新的地址

	// Deprecated
	rgv1.GET("/resource-events", quota.ListEvents)
}

// InitQuotaAdminRouter 运营端 API
func InitQuotaAdminRouter(rgv1 *gin.RouterGroup) {
	rgv1.GET("/quotas/cluster", quota.GetClusterQuota)                          // 查看集群总配额
	rgv1.GET("/quotas/cluster/apu", quota.GetClusterAPU)                        // 查看 ai 加速卡类型
	rgv1.GET("/quotas/cluster/gpu-groups", quota.GetClusterGPUGroup)            // 查看集群GPU分组
	rgv1.PUT("/quotas/cluster/gpu-groups", quota.UpdateClusterGPUGroup)         // 更新集群GPU分组
	rgv1.GET("/quotas/tenants", quota.ListTenantQuotas)                         // 查看所有团队配额
	rgv1.PUT("/quotas/tenants/:tenantName", quota.UpdateTenantQuota)            // 创建或更新某个团队配额
	rgv1.GET("/quotas/tenants/:tenantName", quota.GetTenantDetail)              // 查看某个团队配额
	rgv1.GET("/quotas/tenants/:tenantName/audits", quota.ListTenantQuotaAudits) // 某个团队配额分配历史
	rgv1.GET("/quotas/tenants/:tenantName/users", quota.ListTenantUsers)        // 某个团队成员占用（用户端/运营端）
	rgv1.GET("/quotas/tenants/:tenantName/events", quota.ListEvents)            // 某个团队的历史任务记录（用户端/运营端）
	rgv1.GET("/quotas/tenants/:tenantName/workloads", quota.ListWorkloads)      // 某个团队的当前任务占用

	rgv1.PUT("/quotas/tenants/:tenantName/chargeQuota", quota.UpdateTenantChargedQuota) // 接口更新团队占用配额（比如dataset-storage)

	rgv1.DELETE("/quotas/tenants/:tenantName/users/:userID/release", quota.ReleaseUserResources) // 释放该用户下的所有业务资源 高危操作！！！
}

// InitQuotaRouter 用户端 API
func InitQuotaRouter(rgv1 *gin.RouterGroup) {
	rgv1.GET("/tenants/quotas/detail", quota.GetCurrentTenantDetail)       // 查看当前团队配额
	rgv1.GET("/tenants/quotas/workloads", quota.GetCurrentTenantWorkloads) // 查看当前团队的当前任务占用

	// Deprecated
	rgv1.GET("/tenants/resources/overview", resource.Detail)      // 查看当前团队的资源占用
	rgv1.GET("/tenants/metrics/query_range", resource.GetMetrics) // 查看当前团队的资源监控
	rgv1.GET("/tenants/resources/audits", resource.ListAudit)     // 查看当前团队的事件记录
}

// InitSiteRouter 多集群相关 API
func InitSiteRouter(rgv1 *gin.RouterGroup) {
	rgv1.GET("/sites", site.ListSites)                                    // 查看所有集群Sites
	rgv1.GET("/sites/listproxy", site.ListSiteProxy)                      // 代理业务模块的 API 列表接口
	rgv1.GET("/sites/quotas/cluster", site.SitesClusterQuota)             // 代理业务模块的 API 列表接口
	rgv1.GET("/sites/quotas/cluster/apu", site.SitesClusterAPU)           // 代理业务模块的 API 列表接口
	rgv1.POST("/sites/communication/reportstatus", site.ReportSiteStatus) // 代理业务模块的 API 列表接口
}

func InitAlgoResource(rgv1 *gin.RouterGroup) {
	rgv1.GET("/algoresource/tenants", algoresource.List)
	rgv1.GET("/algoresource/tenants/:tenantID", algoresource.Get)
	rgv1.PUT("/algoresource/tenants/:tenantID", algoresource.Update)
	rgv1.GET("/algoresource/tenants/:tenantID/audits", algoresource.ListAudit)
	rgv1.GET("/algoresource/records/:algoType/:recordID", algoresource.ListDistributedAlgoResourceByID)
	rgv1.DELETE("/algoresource/records/:tenantID/:projectID/:algoType/:recordID", algoresource.DeleteDistributedRecord)
	rgv1.GET("/algoresource/records/tenant/:tenantID/:algoType/:recordID", algoresource.GetDistributedTenantRecord)
	rgv1.DELETE("/algoresource/records/tenant/:tenantID/:algoType/:recordID", algoresource.DeleteDistributedTenantRecord)
}
