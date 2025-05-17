package config

import (
	"sync"
)

var (
	Profile = new(Conf)

	// ColInitMarkers 记录Mongo Col是否初始化的全局对象
	ColInitMarkers = &sync.Map{} // [colName]bool

)

const (
	SdsSuffix = ".sds"

	// MaxSdsLabelCap sdsLabel最大缓存切片长度
	MaxSdsLabelCap = 2048

	// AutoLearnWorkSpace  文件下载临时存放目录
	AutoLearnWorkSpace = "/data/tmp/autolearn/downloads/tenant-%s/project-%s/autolearn-%s/revision-%s"

	// RecommendWorkSpace 文件下载临时存放目录 %s为随机字符串
	RecommendWorkSpace = "/data/tmp/autolearn/downloads/recommend/%s"

	// Nori加速replica数量
	NoriReplicaNum = 2

	// Nori加速状态监测最小检查更新时间 5 * 72 = 360s = 6min 最大检查更新时间 20 * 180 = 3600s = 60min
	MinNoriStateQueryInterval = 5
	MaxNoriStateQueryInterval = 20
	MinNoriStateQueryRetries  = 72
	MaxNoriStateQueryRetries  = 180 // check nori状态最长重试次数

	// autoLearn状态为AutoLearnStateMasterPodCreateSucceeded的最长滞留时间
	AutoLearnCheckTimeout  = 900
	AutoLearnCheckInterval = 10

	// 评测任务检查间隔时间
	EvalJobStateCheckInterval = 10

	// 框标注点的合法数量
	GroundTruthRectPointNum = 2

	// 评测任务Job的默认资源配置
	DefaultCPUPerEvalJob = 8
	DefaultGPUPerEvalJob = 0
	DefaultMemoryEvalJob = 64

	// 日志label
	LogsLabelKey = "brainpp-log-collector"
	LogsLabelVal = "fluent-bit"

	// 字符串 autoLearnName-revisionName 截取长度，因为ais服务中name总长为64， revisionID为15位
	InterceptedLength = 48

	AutoLearnAndRevisionNameRegex = "^[0-9A-Za-z\u4e00-\u9fa5\\.\\-\\_]{1,64}$"
	AutoLearnPrefix               = "autolearn"
	DataSetPrefix                 = "dataset"

	// snapdet自定义设置中(高级模式创建)最大/小worker数(solution数)
	SnapdetMaxWorkerNum = 4
	SnapdetMinWorkerNum = 1

	// TimeLimit保留小数精度
	TimeLimitPrecision = 2

	// ProcessTimePerImg 保留小数精度
	ProcessTimePerImgPrecision = 2

	// snapdet自定义设置中(高级模式创建)最小训练时长, 根据TimeLimitPrecision设定, 单位h
	SnapdetMinTimeLimit = 0.01

	// ScheduleTimeLimit保留小数精度
	ScheduleTimeLimitPrecision = 1

	MaxScheduleTime = 12.0
	MinScheduleTime = 0.2

	// EvalMetric保留小数精度
	EvalMetricPrecision = 5

	// ConpareWithZero 根据EvalMetric设置的小数保留精度设置最小值，根据EvalMetricPrecision设定
	EvalMetricMinValue = 0.00001

	// 检测任务中迭代分数曲线名
	BestDetector = "bestDetector"

	// 分类任务中迭代分数曲线名
	BestClasscifical = "bestClasscifical"

	// 模型评测中meta.json文件名
	ModelMetaFile = "meta.json"

	// 评测指标分数默认值，实际情况最大值可能为0
	EvalMetricDefaultMaxScore = -1

	// 算法分数曲线的下载类型
	AlgorithmScoreTypeMarkdown = "Markdown"
	AlgorithmScoreTypeJSON     = "Json"

	// 模型文件下载超时时间
	ExpireTimeModelFileDownload = 3600

	//  TruncationThresholdPrecision 截断阈值精度
	TruncationThresholdPrecision = 2

	// 新建autoLearn时默认版本名
	DefaultRevisionName string = "v1"

	// OptimizationRvNamePrefix 优化版本名新增后缀
	OptimizationRvNamePrefix = "op-"

	// ReLabelTaskStateCheckInterval 状态查询间隔时间，单位s，后续改为配置
	ReLabelTaskStateCheckInterval = 60 * 3

	// ReLabelTaskStateCheckTime 状态查询总时长，单位s，后续改为配置
	ReLabelTaskStateCheckTime = 3600 * 12

	// MaxEvalComparisonJobNum 实验对比最大评测任务数量
	MaxEvalComparisonJobNum = 20

	// MinEvalComparisonJobNum 实验对比最小评测任务数量
	MinEvalComparisonJobNum = 2

	// AvailableGPUTypeEnv 指定训练任务GPU类型
	AvailableGPUTypeEnv = "AVAILABEL_GPU_TYPES"
)

const (
	// 总成功率 (MetricDetAutoLearnCompleted + MetricClfAutoLearnCompleted) / (MetricDetAutoLearnCreation + MetricClfAutoLearnCreation)
	MetricAutoLearnCreation  = "MAL0001_autolearn_created"   // 实验创建记录
	MetricAutoLearnCompleted = "MAL0002_autolearn_completed" // 实验成功记录

	// 推荐算法成功率 =  Metric(Det/Clf)RecommendSucceeded / Metric(Det/Clf)RecommendRequest
	MetricRecommendRequest   = "MAL0003_recommend_request"   // 推荐算法请求记录
	MetricRecommendSucceeded = "MAL0004_recommend_succeeded" // 推荐算法成功记录

	// 资源调度成功率 = MetricScheduleSucceeded / MetricScheduleRequest
	MetricScheduleRequest   = "MAL0005_ray_cluster_schedule_request"   // 调度请求记录
	MetricScheduleSucceeded = "MAL0006_ray_cluster_schedule_succeeded" // 调度成功记录

	// (Det/Clf)训练成功率 =  Metric(Det/Clf)AutoLearnCompleted / Metric(Det/Clf)AutoLearnLearning
	MetricAutoLearnLearning = "MAL0007_autolearn_learning" // 进入学习中状态记录

	// relabel成功率 = MetricRelabelSucceeded / MetricRelabelRequest
	MetricRelabelRequest   = "MAL0010_relabel_request"   // relabel请求记录
	MetricRelabelSucceeded = "MAL0011_relabel_succeeded" // relabel成功记录

	MetricAutoLearnFailedReason = "MAL0012_autolearn_failed_reason" // 错误原因记录
)
