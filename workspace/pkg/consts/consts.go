package consts

import (
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/authlib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/mgolib"
)

type ctxKey string

var (
	// GinEngine gin框架全局对象
	GinEngine *ginlib.Gin

	// ConfigMap 管理全局配置信息
	ConfigMap = new(Config)

	// MongoClient 本服务mongo client对象
	MongoClient *mgolib.MgoClient

	// Clientsets k8s 相关 client
	Clientsets *Clientset

	// Authenticator authClient全局对象
	Authenticator *authlib.Authenticator

	// SortByDESC 降序
	SortByDESC = "desc"
	// SortByASC 升序
	SortByASC = "asc"
	// PageSize 是常用 query 参数的一部分，用于指明page的大小
	PageSize = "pageSize"
	// Page 是常用 query 参数的一部分，用于指明页码
	Page = "page"
	// SortBy 排序类型
	SortBy = "sortBy"
	// Order 排序字段
	Order = "order"
	Token = "token"

	// Name 资源名
	Name = "name"

	OnlyMe = "onlyMe"

	// WorkspaceID worksapce id
	WorkspaceID = "X-Workspace-ID"

	// InstanceID workspace 实例 id
	InstanceID = "X-Instance-ID"

	// UserID 通过验证后的 user id
	UserID = "X-User-ID"

	// TenantID 通过验证后的 tenant id
	TenantID = "X-Tenant-ID"

	// ProjectID 通过验证后的 project id
	ProjectID = "X-Project-ID"

	Host ctxKey = "x-host"

	App ctxKey = "x-app"

	HeaderToken = "X-Token"

	// KeyType ssh public key type
	KeyType = "X-key-Type"

	// KeyMarshal ssh public key marshal
	KeyMarshal = "X-key-Marshal"

	// KeyInspect ssh public key inspect
	KeyInspect = "X-key-Inspect"

	// InstancePendingDuration instance 的启动时长，单位：秒
	InstancePendingDuration int64 = 10 * 60

	// InstanceDelayDuration intance 的延迟时长，单位：秒
	InstanceDelayDuration int64 = 60

	// workspace 默认配置
	// WorkspacePrefix = "/aapi/workspace.ais.io"
	// DefaultWSImage  = "docker-registry-internal.i.brainpp.cn/brain/ais-workspace:dev"
	// DefaultWSCPU    = "8"
	// DefaultWSMEM    = "32Gi"
	// DefaultWSGPU    = "1"
	// DefaultWSStorage="20Gi"

)

const (
	ExitTimeoutM10 int64 = 600
	ExitTimeoutH1  int64 = 3600
	ExitTimeoutH2  int64 = 7200
	ExitTimeoutH10 int64 = 36000
)

type InstanceState string

const (
	InstanceStatePending   InstanceState = "Pending"
	InstanceStateRunning   InstanceState = "Running"
	InstanceStateCompleted InstanceState = "Completed"
	InstanceStateFailed    InstanceState = "Failed"
)

var (
	KeyInspectTypePositive = "positive"
	KeyInspectTypeNegative = "negative"

	KeyInspectTypes = map[string]bool{
		KeyInspectTypePositive: true,
		KeyInspectTypeNegative: false,
	}
)

const (
	ReasonCodebaseInvalid = "CodebaseInvalid" // 镜像无效
	ReasonNoPermission    = "NoPermission"    // 用户无权限
	ReasonStartupFailed   = "StartupFailed"   // 启动失败
	ReasonScheduleFailed  = "ScheduleFailed"  // 等待资源超时（资源不足）
	ReasonRunningError    = "RunningError"    // 运行出错
	ReasonUnknown         = "Unknown"         // 未知错误
	ReasonOOMKilled       = "OOMKilled"       // 内存不足
	ReasonNotFound        = "NotFound"        // 资源丢失
	ReasonRunningTimeout  = "RunningTimeout"  // 运行超时
	ReasonKilled          = "Killed"          // 被杀死
)
