package types

import (
	evalhubTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/evalhub/pkg/types"
	aisConst "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
)

const CVSimpleDefaultTimeout = 4    // 4 小时
const AIGCSimpleDefaultTimeout = 24 // 12 小时

type AutoLearnCreateRequest struct {
	Name string   `json:"name" validate:"required"` // 不能与现有任务重名，且符合正则表达式："^[0-9A-Za-z\u4e00-\u9fa5\\.\\-\\_]{1,64}$"
	Tags []string `json:"tags"`                     // 标签
	Desc string   `json:"desc"`                     // 描述
	*SameCreateRequest
}

type RevisionCreateRequest struct {
	RevisionName string `json:"revisionName" validate:"required"` // 版本名称，不能与现有任务重名，且符合正则表达式："^[0-9A-Za-z\u4e00-\u9fa5\\.\\-\\_]{1,64}$"
	*SameCreateRequest
}

type SameCreateRequest struct {
	Approach           aisConst.Approach `json:"approach"`
	Type               aisConst.AppType  `json:"type" enums:"Detection,Classification,KPS" validate:"required"` // 场景
	Datasets           []*DataSource     `json:"datasets" validate:"required"`                                  // 数据源信息
	SnapSampler        *SnapSampler      `json:"snapSampler"`                                                   // 训练集采用规则
	CreateMode         CreateMode        `json:"createMode" enums:"Simple,Advanced" validate:"required"`        // 创建模式(简单模式/高级模式)
	SnapdetMode        SnapdetMode       `json:"snapdetMode" enums:"Basic,Standard,Attack"`                     // 学习模式(基础模式/业务标准模式/饱和攻击模式)，简单模式下该值必填
	EvalMetric         *EvalMetric       `json:"evalMetric"`                                                    // 检测类型评测指标，场景为Detection必填
	ClfEvalMetric      *ClfEvalMetric    `json:"clfEvalMetric"`                                                 // 分类类型评测指标，场景为Classification必填
	QuotaGroupID       string            `json:"quotaGroupID"`                                                  // 配额组，如果配额组开启，则该选项必填，例如: "megvii:megvii-face:ais"
	QuotaGroupName     string            `json:"-"`
	PrivateMachine     bool              `json:"-"`                                           // 配额组内是否有私有机器
	TimeLimit          float64           `json:"timeLimit"`                                   // 最大训练时长，单位为小时，切换至高级模式该值必填，简单模式无效
	ScheduleTimeLimit  float64           `json:"scheduleTimeLimit"`                           // 最大等待调度时长，单位小时，默认最小为0.2小时，最大12小时
	WorkerNum          int32             `json:"workerNum"`                                   // 并发训练算法个数，切换至高级模式该值必填，简单模式无效
	GPUPerWorker       int32             `json:"gpuPerWorker"`                                // 每个worker的GPU数量，切换至高级模式该值必填，简单模式无效
	Algorithms         []*Algorithm      `json:"algorithms"`                                  // 从原始算法中选定的推荐算法，切换至高级模式该值必填，简单模式无效
	OriginalAlgorithms []*Algorithm      `json:"originalAlgorithms"`                          // 从推荐系统获取的原始算法, 切换至高级模式该值必填，简单模式无效
	SpeedMetric        *SpeedMetric      `bson:"speedMetric" json:"speedMetric"`              // 速度指标，场景为Detection必填
	ModelType          string            `bson:"modelType" json:"modelType" enums:"MDL,ONNX"` // 模型部署类型
	IsRetry            bool              `json:"isRetry"`                                     // 是否为重试，目前仅内部是否
	SnapXImageURI      string            `json:"snapXImageURI"`                               // 指定训练任务snapX的镜像地址
	SolutionType       SolutionType      `json:"solutionType"`                                // 算法类型 Platform/Custom
	SnapEnvs           []string          `json:"snapEnvs"`                                    // snapX运行时增加的环境变量, key=value
	QuotaPreemptible   string            `json:"quotaPreemptible"`                            // 是否使用抢占式配额
	IsOptimized        bool              `json:"isOptimized"`                                 // 是否优化 create，仅内部使用
	ManagedBy          string            `bson:"managedBy" json:"managedBy"`                  // 指定任务管理人，非必填。默认为"ais.io"
	Continued          bool              `json:"continued"`                                   //  是否为继续学习
	ContinuedMeta      *ContinuedMeta    `json:"continuedMeta"`                               // 继续学习模型+soluiton
	GPUTypes           []string          `bson:"gpuTypes" json:"gpuTypes"`                    // GPU卡类型

	DetectedObjectType aisConst.DetectedObjectType `json:"detectedObjectType" enums:"Normal,MultiMatch"` // 当应用类型为检测时，选择目标类型，普通物体或边界不规则物体
}

type SnapSamplerBuildRequest struct {
	SDSPaths []string         `json:"sdsPaths"`                                                  // sds 文件的绝对路径，需要和最终送到snapdet/clf 的顺序保持严格一致
	Type     aisConst.AppType `json:"type" enums:"Detection,Classification" validate:"required"` // 场景
	Method   MethodType       `json:"method"`                                                    // 按数据集采样：bydataset， 按类别采样 byclass
}

type SnapSamplerUpdateRequest struct {
	Type        aisConst.AppType   `json:"type" enums:"Detection,Classification" validate:"required"` // 场景
	SnapSampler *SnapSampler       `json:"snapSampler"`                                               // 现采用规则
	Weights     map[string]float64 `json:"weights"`                                                   // 原子采样器的权重参数
}

type SnapSampler struct {
	SnapSamplerWeight
	AtomSamplers  []AtomSampler `json:"atomSamplers"`  // 原子采样器列表
	StatisticInfo StatisticInfo `json:"statisticInfo"` // 采样权重更新后，整体数据的分布统计
}

type SnapSamplerWeight struct {
	SnapSamplerInput
	Weights map[string]float64 `json:"weights"` // 原子采样器的权重参数
}

type SnapSamplerInput struct {
	SDSPaths []string   `json:"sdsPaths"` // sds 文件的绝对路径，需要和最终送到snapdet/clf 的顺序保持严格一致
	Method   MethodType `json:"method"`   // 按数据集采样：bydataset， 按类别采样 byclass
	Task     TaskType   `json:"task"`     // 检测任务：det， 分类任务：clf
}

type AtomSampler struct {
	Name          string        `json:"name"`          // 采样器的名字
	Num           int64         `json:"num"`           // 采样器元素数量
	StatisticInfo StatisticInfo `json:"statisticInfo"` // 采样器的基础统计信息
}

type StatisticInfo struct {
	DatasetInfo map[string]float64 `json:"datasetInfo"` // 数据集相关的统计信息
	ClassInfo   map[string]float64 `json:"classInfo"`   // 类别的统计信息
}

type MethodType string

const (
	MethodTypeByDataset MethodType = "bydataset"
	MethodTypeByClass   MethodType = "byclass"
)

type TaskType string

const (
	TaskTypeDet TaskType = "det"
	TaskTypeClf TaskType = "clf"
	TaskTypeKps TaskType = "kps"
	TaskTypeSeg TaskType = "seg"
	TaskTypeOCR TaskType = "ocr"
	TaskTypeLLM TaskType = "llm"
)

var (
	TaskTypeMap = map[aisConst.AppType]TaskType{
		aisConst.AppTypeDetection:      TaskTypeDet,
		aisConst.AppTypeClassification: TaskTypeClf,
		aisConst.AppTypeKPS:            TaskTypeKps,
		aisConst.AppTypeSegmentation:   TaskTypeSeg,
		aisConst.AppTypeOCR:            TaskTypeOCR,
		aisConst.AppTypeText2Text:      TaskTypeLLM,
		aisConst.AppTypeImage2Text:     TaskTypeLLM,
	}
)

type DataSource struct {
	SourceType  SourceType                `json:"sourceType" enums:"Dataset,SDS,Pair" validate:"required"` // 来源类型
	DataURI     string                    `json:"dataURI" validate:"required"`                             // SDS来源：dataURI=s3://bucket/path/to/file.sds Dataset/Pair来源：dataURI=12345,67890(id,revisionID)
	Type        aisConst.DatasetTrainType `json:"type" enums:"Train,Validation"`                           // 数据集类型, 当来源为SourceType=Pair时该值无意义，其他来源时必填
	OriginLevel aisConst.Level            `json:"originLevel" enums:"Public,Project" validate:"required"`  // 数据来源等级，当来源为Dataset、Pair是必填
}

type InternalAgentInfoReq struct {
	AverageRecall   float64     `json:"averageRecall"` // 当前评测指标下所有类别的recall平均值
	SolutionID      string      `json:"solutionID"`
	SolutionName    string      `json:"solutionName"`
	IterationNumber int32       `json:"iterationNumber"`
	PredictSDS      string      `json:"predictSDS"`                                               // sds文件的s3地址，若不为空请确保该oss地址有效性
	ModelFilePath   string      `json:"modelFilePath"`                                            // 模型文件的s3地址，若不为空请确保该oss地址有效性
	State           ActionState `json:"state" enums:"Learning,Completed,Failed,MasterPodRunning"` // 自动学习状态，空表示状态不发生变化，其他值无效
	UpdateTime      int64       `json:"updateTime"`
	EarlyStop       bool        `json:"earlyStop"`
	ErrorCode       int32       `json:"errorCode"` // 训练异常错误代码
	Epoch           int32       `json:"epoch"`     // 模型在算法中产出时的轮次
}

type UpdateRevisionRequest struct {
	State        ActionState      `json:"state" enums:"Stopped"` // 自动学习状态，只支持stop操作
	RevisionName string           `json:"revisionName"`          // 版本名称，不能与现有任务重名，且符合正则表达式："^[0-9A-Za-z\u4e00-\u9fa5\\.\\-\\_]{1,64}$"
	Visibility   VisibilityOption `json:"visibility"`
}

type ActionState string

const (
	ActionMasterPodCheckFailed ActionState = "MasterPodCheck"
	ActionMasterPodRunning     ActionState = "MasterPodRunning"
	ActionAJobCreated          ActionState = "JobCreated"
	ActionStateLearning        ActionState = "Learning"
	ActionStateCompleted       ActionState = "Completed"
	ActionStateFailed          ActionState = "Failed"
	ActionStateStopped         ActionState = "Stopped"
)

type VisibilityOption = string

const (
	VisibilityOptionHidden  VisibilityOption = "hidden"
	VisibilityOptionVisible VisibilityOption = "visible"
)

type RecommendSolutionRequest struct {
	Type         aisConst.AppType `json:"type" enums:"Detection,Classification" validate:"required"` // 场景
	Datasets     []*DataSource    `json:"datasets" validate:"required"`                              // datasets 数据源信息
	TimeLimit    float64          `json:"timeLimit" default:"4"`                                     // 最大训练时长，单位为小时，切换至高级模式该值必填
	WorkerNum    int32            `json:"workerNum"`                                                 // 并发训练算法个数，切换至高级模式该值必填
	*SpeedMetric                  // 速度指标
	ModelType    string           `json:"modelType"`    // 模型部署类型
	SolutionName string           `json:"solutionName"` // 继续学习指定的 solution
}

type EvalJobWithoutFinalState struct {
	AutoLearn struct {
		ProjectID     string `bson:"projectID"`
		TenantID      string `bson:"tenantID"`
		AutoLearnID   string `bson:"autoLearnID"`
		AutoLearnName string `bson:"autoLearnName"`
	} `bson:"autoLearn"`
	AutoLearnRevisions struct {
		RevisionID       string            `bson:"revisionID"`
		RevisionName     string            `bson:"revisionName"`
		Type             string            `bson:"type"`
		CreatedBy        *User             `bson:"createdBy"`
		EvaluationDetail *EvaluationDetail `bson:"evaluationDetail"`
	} `bson:"autoLearnRevisions"`
}

type DatasetAttributeRequest struct {
	Dataset *DataSource `json:"dataset" validate:"required"` // 数据源信息
}

type DatasetsAttributeRequest struct {
	Datasets []*DataSource `json:"datasets" validate:"required"` // 数据源信息
}

type ExportModelRequest struct {
	IterationNumber     int32            `json:"iterationNumber"`
	ImageHeight         int32            `json:"imageHeight"`                      // 图片高度
	ImageWidth          int32            `json:"imageWidth"`                       // 图片宽度
	ImageInputFormat    ImageInputFormat `json:"imageInputFormat" enums:"BGR,YUV"` // 图片格式，如BGR等
	TruncationThreshold float64          `json:"truncationThreshold"`              // 截断阈值
	SourceModelFile     string           `json:"-"`                                // 原始模型文件oss路径
}

type EvalComparisonReq struct {
	Type                aisConst.AppType `json:"type" enums:"Detection"`
	ComparisonRevisions []*alAndRvID     `json:"comparisonRevisions"`
}

type alAndRvID struct {
	AutoLearnID string `json:"autoLearnID"`
	RevisionID  string `json:"revisionID"`
}

type EvalComparisonSnapshotReq struct {
	EvalComparisonID string                                        `json:"evalComparisonID"` // 自动学习服务的评测对比ID
	SnapshotInfo     *evalhubTypes.CreateComparisonSnapshotRequest `json:"snapshotInfo"`     // 和评测对比任务中[POST /v1/comparison-snapshots]参数一致
}

type ExportComparisonSnapshotReq struct {
	EvalRequest *evalhubTypes.ExportComparisonSnapshotRequest `json:",inline"`
}
