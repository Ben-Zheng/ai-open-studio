package types

import (
	"go.mongodb.org/mongo-driver/bson/primitive"

	evalTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/evalhub/pkg/types"
	aisConst "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
)

type AutoLearn struct {
	TenantID           string               `bson:"tenantID" json:"tenantID"`   // 租户名
	ProjectID          string               `bson:"projectID" json:"projectID"` // 项目名
	ProjectName        string               `bson:"projectName" json:"projectName"`
	AutoLearnID        string               `bson:"autoLearnID" json:"autoLearnID"`     // 自动学习任务 ID
	AutoLearnName      string               `bson:"autoLearnName" json:"autoLearnName"` // 任务名称
	Approach           aisConst.Approach    `bson:"approach" json:"approach"`
	Type               aisConst.AppType     `bson:"type" json:"type"`           // 场景
	Desc               string               `bson:"desc" json:"desc"`           // 描述
	Tags               []string             `bson:"tags" json:"tags"`           // 标签
	CreatedBy          *User                `bson:"createdBy" json:"createdBy"` // 创建者
	UpdatedBy          *User                `bson:"updatedBy" json:"updatedBy"` // 更新者
	CreatedAt          int64                `bson:"createdAt" json:"createdAt"` // 创建时间
	UpdatedAt          int64                `bson:"updatedAt" json:"updatedAt"` // 更新时间
	Continued          bool                 `bson:"continued" json:"continued"` // 继续学习任务
	IsDeleted          bool                 `bson:"isDeleted" json:"-"`
	AutoLearnRevisions []*AutoLearnRevision `bson:"autoLearnRevisions" json:"autoLearnRevisions"` // 学习任务版本

	// 返回给前端，不存数据库
	RevisionsStateStat []*RevisionsStateStat `bson:"-" json:"revisionsStateStat"` // 版本状态统计
}

type AutoLearnLastRevision struct {
	*AutoLearnRevision `json:",inline" bson:",inline"`
	AutoLearnName      string `json:"autoLearnName" bson:"autoLearnName"`
	TenantID           string `json:"tenantID" bson:"tenantID"`
	ProjectID          string `json:"projectID" bson:"projectID"`
	ProjectUID         string `json:"projectUID"`  // 仅前端使用
	ProjectName        string `json:"projectName"` // 仅前端使用
}

type AutoLearnRevision struct {
	TenantID             string                  `bson:"-" json:"-"`
	ProjectID            string                  `bson:"-" json:"-"`
	AutoLearnID          string                  `bson:"autoLearnID" json:"autoLearnID"`     // 自动学习任务 ID
	AutoLearnName        string                  `bson:"autoLearnName" json:"autoLearnName"` // 自动学习任务名称
	RevisionID           string                  `bson:"revisionID" json:"revisionID"`       // 版本 ID
	RevisionName         string                  `bson:"revisionName" json:"revisionName"`   // 版本名称
	Approach             aisConst.Approach       `bson:"approach" json:"approach"`
	Type                 aisConst.AppType        `bson:"type" json:"type"`                                        // 场景
	Datasets             []*Dataset              `bson:"datasets" json:"datasets"`                                // 数据集
	SnapSampler          *SnapSampler            `bson:"snapSampler" json:"snapSampler"`                          // 训练集采用规则
	TimeLimit            float64                 `bson:"timeLimit" json:"timeLimit"`                              // 最大训练时长
	ScheduleTimeLimit    float64                 `bson:"scheduleTimeLimit" json:"scheduleTimeLimit"`              // 可等待调度时长，单位小时
	QuotaGroup           string                  `bson:"quotaGroup" json:"quotaGroup"`                            // 配额组ID
	QuotaGroupName       string                  `bson:"-" json:"-"`                                              // 配额组名字
	CreateMode           CreateMode              `json:"createMode" enums:"Simple, Advanced" validate:"required"` // 创建模式(简单模式/高级模式)
	SnapdetMode          SnapdetMode             `bson:"snapdetMode" json:"snapdetMode"`                          // 学习模式
	ModelType            string                  `bson:"modelType" json:"modelType"`
	Resource             *Resource               `bson:"resource" json:"resource"`                         // 版本占用资源
	EvalMetric           *EvalMetric             `bson:"-" json:"evalMetric"`                              // 检测评测指标
	ClfEvalMetric        *ClfEvalMetric          `bson:"-" json:"clfEvalMetric"`                           // 分类评测指标
	F1EvalMetric         *EvalMetric             `bson:"-" json:"f1EvalMetric"`                            // 检测评测指标
	SpeedMetric          *SpeedMetric            `bson:"speedMetric" json:"speedMetric"`                   // 速度指标
	EvaluationDetails    []*EvaluationDetail     `bson:"-" json:"evaluationDetail"`                        // 版本详情
	Algorithms           []*Algorithm            `bson:"-" json:"algorithms"`                              // 经修改后实际使用的算法
	OriginalAlgorithms   []*Algorithm            `bson:"-" json:"originalAlgorithms"`                      // 从推荐系统获取的算法
	AutoLearnState       AutoLearnState          `bson:"autoLearnState" json:"autoLearnState"`             // 自动学习状态
	AutoLearnStateStream []*AutoLearnStateStream `bson:"autoLearnStateStream" json:"autoLearnStateStream"` // 自动学习状态流
	EarlyStop            bool                    `bson:"earlyStop" json:"earlyStop"`                       // 学习曲线是否平滑
	Reason               string                  `bson:"reason" json:"reason"`                             // 自动学习初始化失败、异常原因记录
	CreatedBy            *User                   `bson:"createdBy" json:"createdBy"`                       // 创建者
	UpdatedBy            *User                   `bson:"updatedBy" json:"updatedBy"`                       // 更新者
	CreatedAt            int64                   `bson:"createdAt" json:"createdAt"`                       // 创建时间
	UpdatedAt            int64                   `bson:"updatedAt" json:"updatedAt"`                       // 更新时间
	LogsPodName          string                  `bson:"logsPodName" json:"logsPodName"`                   // 日志pods名字
	IsDeleted            bool                    `bson:"isDeleted" json:"-"`
	IsOptimized          bool                    `bson:"isOptimized" json:"isOptimized"`     // 是否为优化版本
	ReLabelTaskID        primitive.ObjectID      `bson:"reLabelTaskID" json:"reLabelTaskID"` // 训练集优化任务ID
	SnapXImageURI        string                  `bson:"snapXImageURI" json:"snapXImageURI"` // 当前训练任务snapX的镜像地址
	SolutionType         SolutionType            `bson:"solutionType" json:"solutionType"`   // 当前任务算法类型
	IsHidden             bool                    `bson:"isHidden" json:"-"`                  // 该实验是否为隐藏
	SnapEnvs             []string                `bson:"snapEnvs" json:"snapEnvs"`           // snapX运行时增加的环境变量
	QuotaPreemptible     string                  `json:"quotaPreemptible"`                   // 是否使用抢占式配额
	ManagedBy            string                  `bson:"managedBy" json:"managedBy"`         // 指定任务管理人，非必填。默认为"ais.io"
	Continued            bool                    `bson:"continued" json:"continued"`         // 是否为继续学习的任务
	ContinuedMeta        *ContinuedMeta          `bson:"continuedMeta" json:"continuedMeta"` // 继续学习算法和模型
	GPUTypes             []string                `bson:"gpuTypes" json:"gpuTypes"`           // GPU卡类型

	DetectedObjectType aisConst.DetectedObjectType `bson:"detectedObjectType" json:"detectedObjectType"` // 当应用类型为检测时，选择目标类型，普通物体或边界不规则物体

	// 返回给前端，不存数据库
	ConvertToStateSchedulingTime int64         `bson:"-" json:"convertToStateSchedulingTime"` // 转为调度中(待学习)状态时间
	ConvertToStateLearningTime   int64         `bson:"-" json:"convertToStateLearningTime"`   // 转为学习中状态的时间
	TimeConsumed                 string        `bson:"-" json:"timeConsumed"`                 // 任务耗时，格式HH:mm:ss
	EvaluationJobsStateInRunning []int32       `bson:"-" json:"evaluationJobsStateInRunning"` // 处于评测中的评测子任务，列举迭代号
	BestDetector                 *BestDetector `bson:"-" json:"bestDetector"`                 // Algorithms中具有迭代次数score值集合
}

// 表二：autoLearn evaluation评测信息表，用于存储评测指标和详情(这个比较独立)，cloName: <tenantName>-evaluation
type AutoLearnEvaluation struct {
	TenantID          string              `bson:"-" json:"-"`
	ProjectID         string              `bson:"projectID" json:"projectID"`                 // 项目名
	AutoLearnID       string              `bson:"autoLearnID" json:"autoLearnID"`             // 自动学习任务ID
	RevisionID        string              `bson:"revisionID" json:"revisionID"`               // 版本ID
	EvalMetric        *EvalMetric         `bson:"evalMetric" json:"evalMetric"`               // 检测评测指标
	ClfEvalMetric     *ClfEvalMetric      `bson:"clfEvalMetric" json:"clfEvalMetric"`         // 分类评测指标
	F1EvalMetric      *EvalMetric         `bson:"f1EvalMetric" json:"f1EvalMetric"`           // f1评测指标
	EvaluationDetails []*EvaluationDetail `bson:"evaluationDetails" json:"evaluationDetails"` // 版本详情
}

// 表三：autoLearn algorithm算法表，用于存储每个实验的源推荐算法和实际使用推荐算法(因为信息太多)， cloName: <tenantName>-algorithm
type AutoLearnAlgorithm struct {
	AutoLearnID        string       `bson:"autoLearnID" json:"autoLearnID"`               // 自动学习任务ID
	RevisionID         string       `bson:"revisionID" json:"revisionID"`                 // 版本ID
	ActualAlgorithms   []*Algorithm `bson:"actualAlgorithms" json:"ActualAlgorithms"`     // 经修改后实际使用的算法
	OriginalAlgorithms []*Algorithm `bson:"originalAlgorithms" json:"originalAlgorithms"` // 从推荐系统获取的算法
}

// collection: <tenantName>-eval_comparison
type EvalComparison struct {
	ID           primitive.ObjectID   `json:"id" bson:"_id"`
	ComparisonID primitive.ObjectID   `bson:"comparisonID" json:"comparisonID"` // 来自于评测对比
	ProjectID    string               `bson:"projectID" json:"-"`
	TenantID     string               `bson:"tenantID" json:"-"`
	AppType      aisConst.AppType     `bson:"appType" json:"appType"`
	EvalJobs     []*ComparisonEvalJob `bson:"evalJobs" json:"evalJobs"`
	IsDeleted    bool                 `bson:"isDeleted" json:"-"`
}

// collection: <tenantName>-eval_comparison_snapshot
type EvalComparisonSnapshot struct {
	ID                   primitive.ObjectID   `bson:"_id" json:"id" `
	AppType              aisConst.AppType     `bson:"appType" json:"appType" enums:"Classification,Detection"` // 类型，分类或者检测
	ProjectID            string               `bson:"projectID" json:"-"`
	TenantID             string               `bson:"tenantID" json:"-"`
	EvalComparisonID     primitive.ObjectID   `bson:"evalComparisonID" json:"evalComparisonID"`
	ComparisonID         primitive.ObjectID   `bson:"comparisonID" json:"comparisonID"`                 // 来自于评测对比
	ComparisonSnapshotID primitive.ObjectID   `bson:"comparisonSnapshotID" json:"comparisonSnapshotID"` // 来自于评测对比
	CreatedBy            *User                `bson:"createdBy" json:"createdBy"`
	CreatedAt            int64                `bson:"createdAt" json:"createdAt" `
	UpdatedAt            int64                `bson:"updatedAt" json:"updatedAt" `
	IsDeleted            bool                 `bson:"isDeleted" json:"-" `
	EvalJobs             []*ComparisonEvalJob `bson:"-" json:"evalJobs"` // 评测子任务
}

// ComparisonEvalJob 保存评测子任务对应版本、迭代等相关信息
type ComparisonEvalJob struct {
	AutoLearnID        string `bson:"autoLearnID" json:"autoLearnID"`
	AutoLearnName      string `bson:"autoLearnName" json:"autoLearnName"`
	RevisionID         string `bson:"revisionID" json:"revisionID"`
	RevisionName       string `bson:"revisionName" json:"revisionName"`
	IterationNumber    int32  `bson:"iterationNumber" json:"iterationNumber"`
	EvaluationID       string `bson:"evaluationID" json:"evaluationID"` // 评测任务ID
	EvaluationJobID    string `bson:"evaluationJobID" json:"evaluationJobID"`
	EvaluationRJobName string `bson:"evaluationRJobName" json:"evaluationRJobName"` // 评测任务RJob名称
	// 这些字段用于实验对比导出展示
	Tenant       string       `bson:"-" json:"-"`
	Project      string       `bson:"-" json:"-"`
	Datasets     []*Dataset   `bson:"-" json:"-"` // 实验对应的训练集和验证集
	ModelFileURI string       `bson:"-" json:"-"` // 产出模型地址
	Algorithms   []*Algorithm `bson:"-" json:"-"` // 算法配置名称                            // 经修改后实际使用的算法
}

type ImageInputFormat string

const (
	ImageInputFormatBGR ImageInputFormat = "BGR"
	ImageInputFormatYUV ImageInputFormat = "YUV"
)

type User struct {
	UserID   string `bson:"userID" json:"userID"`
	UserName string `bson:"userName" json:"userName"`
}

type Dataset struct {
	SourceType           SourceType                `bson:"sourceType" json:"sourceType" enums:"Dataset, SDS, Pair" validate:"required"` // 来源类型
	Type                 aisConst.DatasetTrainType `bson:"type" json:"type" enums:"Train, Validation"`                                  // 数据集类型
	SDSPath              string                    `bson:"sdsPath" json:"sdsPath" example:"s3://bucket/path/to/sds/file.sds"`           // SDS后缀文件的oss地址
	DatasetMeta          *DatasetMeta              `bson:"datasetMeta" json:"datasetMeta"`
	Volume               map[uint64]string         `bson:"volume" json:"-"`        // 当前数据集包含的nori(volumeID >>> noriPath)
	Classes              []string                  `bson:"classes" json:"classes"` // 当前数据集包含的的属性类型
	IsConverstation      bool                      `bson:"_" json:"_"`
	HasNilBoxes          bool                      `bson:"-" json:"-"` // 存在boxes字段为nil的item
	NilKeypoints         bool                      `bson:"-" json:"-"` // keypoint整体为nil
	HasNilOrEmptyClasses bool                      `bson:"-" json:"-"` // 存在Classes字段为nil或空的item
	HasRegressor         bool                      `bson:"-" json:"-"` // 存在 regressors 字段不为空
	HasClasses           bool                      `bson:"-" json:"-"` // 存在 classess 字段不为空
	HasTexts             bool                      `bson:"-" json:"-"` // 存在 texts 字段可以为空
	ParseFailed          bool                      `bson:"-" json:"-"` // sds文件解析是否成功
	RecommendClasses     *RecommendClasses         `bson:"-" json:"-"` // 简单模式推荐算法依赖的属性统计
}

type SourceType string

const (
	SourceTypeDataset SourceType = "Dataset"
	SourceTypeSDS     SourceType = "SDS"
	SourceTypePair    SourceType = "Pair"
)

type DatasetMeta struct {
	ID                 string         `bson:"id" json:"id" validate:"required"`                                          // 数据集 ID
	Name               string         `bson:"name" json:"name"`                                                          // 数据集名
	RevisionID         string         `bson:"revisionID" json:"revisionID" validate:"required"`                          // 数据集版本ID
	RevisionName       string         `bson:"revisionName" json:"revisionName"`                                          // 数据集版本名
	PairID             string         `bson:"pairID" json:"pairID"`                                                      // 训验对ID
	PairName           string         `bson:"pairName" json:"pairName"`                                                  // 训验对名
	PairRevisionID     string         `bson:"pairRevisionID" json:"pairRevisionID"`                                      // 训验对版本ID
	PairRevisionName   string         `bson:"pairRevisionName" json:"pairRevisionName"`                                  // 训验对版本名
	OriginLevel        aisConst.Level `bson:"originLevel" json:"originLevel" enums:"Public,Project" validate:"required"` // 数据来源等级，公共数据集或项目内数据集,当选择平台内数据时具备该属性
	MSGPath            string         `bson:"-" json:"-"`                                                                // 数据集的entity.msg文件的S3路径
	ExternalDateSetID  string         `bson:"externalDateSetID" json:"externalDateSetID" validate:"required"`            // 标注平台数据集 ID
	ExternalRevisionID string         `bson:"externalRevisionID" json:"externalRevisionID" validate:"required"`          // 标注平台数据集版本 ID
	BatchID            string         `bson:"batchID" json:"batchID" validate:"required"`                                // 标注平台数据集批次 ID
	InstitutionID      string         `bson:"institutionID" json:"institutionID" validate:"required"`                    // 机构 ID
}

type CreateMode string

const (
	CreateModeSimple  CreateMode = "Simple"
	CreateModeAdvance CreateMode = "Advanced"
)

type (
	SnapMode    string
	SnapclfMode = SnapMode
	SnapdetMode = SnapMode
)

const (
	SnapdetModeBasic    SnapMode = "Basic"
	SnapdetModeStandard SnapMode = "Standard"
	SnapdetModeAttack   SnapMode = "Attack"
)

const (
	SnapclfModeBasic    = SnapdetModeBasic
	SnapclfModeStandard = SnapdetModeStandard
)

type Resource struct {
	GPU       int32 `json:"gpu" bson:"gpu"`
	CPU       int32 `json:"cpu" bson:"cpu"`
	Memory    int32 `json:"memory" bson:"memory"`
	WorkerNum int32 `json:"workerNum" bson:"workerNum"` // 并发训练算法个数
}

type ProposedResource struct {
	TrainRequestsResources RequestResource
	InferRequestsResources RequestResource
}

type RequestResource struct {
	CPU         int     `json:"cpu"`
	Memory      float64 `json:"memory"`
	GPU         int     `json:"gpu"`
	GPUName     string  `json:"gpu_name"`      // ais 中的 device label
	GPUTypeKey  string  `json:"gpu_type_key"`  // NPUType, GPUType
	ResouceName string  `json:"resource_name"` // pod request GPU 关键字，如： nvidia.com/gpu
}

type EvalMetric struct {
	Key       EvalMetricKey `bson:"key" json:"key" validate:"required"`
	IOU       float64       `bson:"iou" json:"iou"`
	Distance  float64       `bson:"distance" json:"distance"`
	Condition *Condition    `bson:"condition" json:"condition" validate:"required"` // 评测指标基于的其他条件
	MaxScore  float64       `bson:"maxScore" json:"maxScore"`                       // 训练过程中所有算法Socre中实时最大值
}

type Condition struct {
	Key   EvalMetricKey `bson:"key" json:"key" enums:"recall,fp,precision,fppi,confidenceThreshold" validate:"required"`
	Value any           `bson:"value" json:"value" validate:"required"`
}

type ClfEvalMetric struct {
	Key        EvalMetricKey `bson:"key" json:"key" enums:"accuracy,mean_f1_score,mean_recall,mean_precision"`
	Attributes []string      `bson:"attributes" json:"attributes"` // 选择的类别
	MaxScore   float64       `bson:"maxScore" json:"maxScore"`
	Value      any           `bson:"value" json:"value"`
}

type EvalMetricKey string

const (
	// 检测类型评测指标
	EvalMetricKeyRecall     EvalMetricKey = "recall"
	EvalMetricKeyFP         EvalMetricKey = "fp"
	EvalMetricKeyPrecision  EvalMetricKey = "precision"
	EvalMetricKeyFppi       EvalMetricKey = "fppi"
	EvalMetricKeyConfidence EvalMetricKey = "confidence"
	EvalMetricKeyMIOU       EvalMetricKey = "miou"

	// 分类类型评测指标
	EvalMetricKeyAccuracy         EvalMetricKey = "accuracy"
	EvalMetricKeyMeanF1Score      EvalMetricKey = "mean_f1_score"
	EvalMetricKeyMeanRecall       EvalMetricKey = "mean_recall"
	EvalMetricKeyMeanPrecision    EvalMetricKey = "mean_precision"
	EvalMetricKeyPrecisionRecall  EvalMetricKey = "precision@recall"
	EvalMetricKeyRecallPrecision  EvalMetricKey = "recall@precision"
	EvalMetricKeyRecallConfidence EvalMetricKey = "recall@confidence threshold"

	// 回归任务评测指标
	EvalMetricKeyAccuracyL1     EvalMetricKey = "accuracy_l1_mean"
	EvalMetricKeyAccuracyBlurL1 EvalMetricKey = "pb"
)

type EvaluationJobDetail struct {
	EvaluationJobID     string                       `bson:"evaluationJobID" json:"evaluationJobID"`
	EvaluationRJobName  string                       `bson:"evaluationRJobName" json:"evaluationRJobName"`
	EvaluationJAobState evalTypes.EvalJobResultState `bson:"evaluationJobState" json:"evaluationJobState" enums:"Running,Successed,Failed"` // 评测任务Job状态
	AppType             aisConst.AppType             `bson:"appType" json:"appType"`
	Reason              string                       `bson:"reason" json:"reason"` // 失败原因
}

type EvaluationDetail struct {
	IterationNumber    int32                        `bson:"iterationNumber" json:"iterationNumber"`                                        // 迭代次数
	EvaluationJobState evalTypes.EvalJobResultState `bson:"evaluationJobState" json:"evaluationJobState" enums:"Running,Successed,Failed"` // 评测任务Job状态
	EvaluationID       string                       `bson:"evaluationID" json:"evaluationID"`                                              // 评测任务ID
	EvaluationName     string                       `bson:"evaluationName" json:"evaluationName"`                                          // 评测任务名称
	EvaluationJobID    string                       `bson:"evaluationJobID" json:"evaluationJobID"`                                        // 评测任务jobID
	EvaluationRJobName string                       `bson:"evaluationRJobName" json:"evaluationRJobName"`                                  // 评测任务RJob名称
	EvaluationJobs     []*EvaluationJobDetail       `bson:"evaluationJobs" json:"evaluationJobs"`
	SdsFilePath        string                       `bson:"sdsFilePath" json:"sdsFilePath"`     // sds文件oss路径
	ModelFilePath      string                       `bson:"modelFilePath" json:"modelFilePath"` // 模型文件oss路径
	Reason             string                       `bson:"reason" json:"reason"`               // 失败原因
	IsDeleted          bool                         `bson:"isDeleted" json:"-"`
	SolutionType       SolutionType                 `bson:"-" json:"solutionType"`
	SolutionImageURI   string                       `bson:"-" json:"solutionImageURI"`
	AlgorithmName      string                       `bson:"-" json:"algorithmName"`
	Epoch              int32                        `bson:"epoch" json:"epoch"` // 此模型产出的轮次
}

// Algorithm 算法定义
type Algorithm struct {
	ID               string            `bson:"id" json:"id"`
	Name             string            `bson:"name" json:"name"`
	SuperName        string            `bson:"superName" json:"superName"`
	BackboneName     string            `bson:"backboneName" json:"backboneName"`
	Score            []*Score          `bson:"score" json:"score"`
	Properties       []*Property       `bson:"properties" json:"properties"`
	Property         *PropertyV1       `bson:"property" json:"-"` // 自v2.0开始，已废弃
	HyperParam       []*HyperParam     `bson:"hyperParam" json:"hyperParam"`
	HyperParamDefine *HyperParamDefine `json:"hyperParamDefine" bson:"-"`
	SolutionType     SolutionType      `bson:"solutionType" json:"solutionType"` // 此算法的类型
	ImageURI         string            `bson:"imageURI" json:"imageURI"`         // 自定义算法镜像地址
	Description      string            `bson:"description" json:"description"`
	OriginalInfo     string            `bson:"originalInfo" json:"originalInfo"` // 推荐系统返回的其他非展示信息，但是进行训练时需要使用
	Envs             map[string]any    `bson:"envs" json:"envs"`
	ProposedResource *ProposedResource `bson:"proposedResource" json:"proposedResource"` // 综合推荐资源后的最终资源，体现在 solutionConfig 中
}

type AlgorithmSolutionhub struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Version   string   `json:"version"`
	Type      string   `json:"type"`
	Image     []*Score `json:"score"`
	ModelInfo struct {
		SuperName    string `json:"super_name"`
		BackboneName string `json:"backbone_name"`
	} `json:"modelInfo"`
	HyperParamDefine string         `json:"hyperParamDefine"`
	Description      string         `json:"description"`
	Profiling        map[string]any `json:"profiling"`
}

type AlgorithmSolutionhubProfiling struct {
	Device       string  `json:"device"`
	ImageSpecs   string  `json:"image_specs"`
	InferTime    float64 `json:"infer_time"`
	MaxDeviceMem float64 `json:"max_device_mem"`
}

// Score 算法对应的学习曲线
type Score struct {
	Time            int64   `bson:"time" json:"time"`
	Value           float64 `bson:"value" json:"value"`
	IterationNumber int32   `bson:"iterationNumber" json:"iterationNumber"`
	Epoch           int32   `bson:"epoch" json:"epoch"`
}

// Property 算法属性描述
type Property struct {
	Key         string  `bson:"key" json:"key"`                 // 属性名
	Value       float64 `bson:"value" json:"value"`             // 属性值
	Unit        string  `bson:"unit" json:"unit"`               // 单位
	CPU         string  `bson:"cpu" json:"cpu"`                 // cpu型号， 如Intel(R)Xeon(R)Gold6130, 某些属性该值为空
	GPU         string  `bson:"gpu" json:"gpu"`                 // gpu型号， 如T4 / P4 / 2080Ti，某些属性该值为空
	Description string  `bson:"description" json:"description"` // 描述信息
	ImageSpec   string  `bson:"imageSpec" json:"imageSpec"`     // 图片大小， 如512x512 / 768x1280 / 1080x1920， 某些属性该值为空
}

// Property 算法属性描述  // 自v2.0开始，已废弃
type PropertyV1 struct {
	ImageSpec   string         `bson:"imageSpec" json:"imageSpec"`     // 基于的图片大小
	CPUModel    string         `bson:"cpuModel" json:"cpuModel"`       // 基于的CPU型号
	GPUModel    string         `bson:"gpuModel" json:"gpuModel"`       // 基于的GPU信号
	Performance []*Performance `bson:"performance" json:"performance"` // 算法性能
}

// Performance 基于Property中的图片和硬件型号得到的算法性能，如macs、latency、latency_onnx
type Performance struct {
	Key         PerformanceKey `bson:"key" json:"key" enums:"MACs,latency,latency_onnx,modelSize"`
	Value       float64        `bson:"value" json:"value"`
	Unit        string         `bson:"unit" json:"unit"`
	Description string         `bson:"-" json:"description"`
}

// HyperParam 算法中超参，如basicLearnRate、batchSize、Epoch等等
type HyperParam struct {
	Key         string `bson:"key" json:"key"`
	SnapdetKey  string `bson:"snapdetKey" json:"snapdetKey"`
	Value       any    `bson:"value" json:"value"`
	Description string `bson:"description" json:"description"`
}

type PerformanceKey string

const (
	AlgorithmPerformanceMACs        PerformanceKey = "MACs"
	AlgorithmPerformanceLatency     PerformanceKey = "latency"
	AlgorithmPerformanceLatencyOnnx PerformanceKey = "latency_onnx"
	AlgorithmPerformanceModelSize   PerformanceKey = "modelSize"
)

type AutoLearnStateStream struct {
	AutoLearnState AutoLearnState `bson:"autoLearnState" json:"autoLearnState"`
	CreatedAt      int64          `bson:"createdAt" json:"createdAt"`
}

type RevisionsStateStat struct {
	State AutoLearnState `bson:"-" json:"state" enums:"0,1,2,3,4,5,6"` // 初始化中，待学习，学习中，学习完成，学习停止，初始化失败，学习异常
	Num   int32          `bson:"-" json:"num"`
}

type BestDetector struct {
	Name                string               `bson:"-" json:"name"`
	BestDetectorScore   []*BestDetectorScore `bson:"-" json:"bestDetectorScore"`
	BestIterationNumber int32                `bson:"-" json:"bestIterationNumber"`
}
type BestDetectorScore struct {
	AlgorithmName string `bson:"-" json:"algorithmName"`
	*Score        `bson:"-"`
}

type SpeedMetric struct {
	GPUModel          GPUModelType  `bson:"gpuModel" json:"gpuModel" enums:"T4,2080Ti"`                    // gpu型号， 如T4 / P4 / 2080Ti
	ImageSpec         ImageSpecType `bson:"imageSpec" json:"imageSpec" enums:"768x1280,1080x1920,512x512"` // 图片尺寸
	ProcessTimePerImg *float64      `bson:"processTimePerImg" json:"processTimePerImg"`                    // 单张图片计算处理时间, 单位ms
}

type GPUModelType string

const (
	GPUModelTypeT4     GPUModelType = "T4"
	GPUModelType2080Ti GPUModelType = "2080Ti"
)

type ImageSpecType string

const (
	ImageSpecType521  ImageSpecType = "512x512"
	ImageSpecType768  ImageSpecType = "768x1280"
	ImageSpecType1080 ImageSpecType = "1080x1920"
)

type DatasetAttributeType string

const (
	DatasetAttributeTypeSingleAttr     = "SingleAttribute"
	DatasetAttributeTypeMultiAttribute = "MultiAttribute"
)

const (
	KubebrainToolsVolumeName = "kubebrain-tools"
	KubebrainToolsDir        = "/opt/kubebrain/client-tools"
	KubebrainToolsMountDir   = "/tools"

	InternalAgentImageBinaryDir      = "/usr/local/bin" // internal agent镜像内的binary path
	InternalAgentImageBinaryName     = "internal-agent" // internal agent镜像内的binary name
	SnapXImageInternalAgentBinaryDir = "/autolearn/"    // snapx image中把internal agent binary所在的目录

	InitContainerName            = "add-autolearn-binary"
	InitContainerShareVolumeName = "sharedir"

	WorkerInitCMD    = "/tools/worker-init"
	InternalAgentCMD = "/autolearn/internal-agent"

	AutoLearnNameKey             = "autolearn.brainpp.cn/autolearn-name"
	DefaultGangSchedulerLabelKey = "scheduling.kubebrain.brainpp.cn/podgroup_name"
	AutolearnTypeKey             = "autolearn.brainpp.cn/autolearn-type"
	AutolearnTypeTaskValue       = "task"
	AutolearnTypeWorkerValue     = "worker"
)

type SolutionType string

const (
	SolutionTypePlatform SolutionType = "Platform"
	SolutionTypeCustom   SolutionType = "Custom"
)

type ContinuedRevisionModel struct {
	RevisionID      string `bson:"revisionID" json:"revisionID"`
	ModelPath       string `bson:"modelPath" json:"modelPath"`
	IterationNumber int    `bson:"iterationNumber" json:"iterationNumber"`
}

type ContinuedMeta struct {
	ModelID         string   `bson:"modelID" json:"modelID"`
	ModelRevisionID string   `bson:"modelRevisionID" json:"modelRevisionID"`
	ModelPath       string   `bson:"modelPath" json:"modelPath"`
	SolutionName    string   `bson:"solutionName" json:"solutionName"`
	Classes         []string `bson:"classes" json:"classes"`
}

type HyperParamDefine struct {
	Type       string                    `json:"type"`
	Properties map[string]map[string]any `json:"properties"`
}

type HyperParamDefineProperty struct {
	Default     any    `json:"default"`
	Description string `json:"description"`
	Title       string `json:"title"`
}

type RecommendClasses struct {
	Boxes         map[string]int64
	Regressors    map[string]int64
	Classes       map[string]int64
	Keypoints     map[string]int64
	Segmentations map[string]int64
	Texts         map[string]int64
	Lines         map[string]int64
	Count         int
}

type RecommendRequestBody struct {
	Type            string                  `json:"type"`
	SolutionNumber  int                     `json:"topk"`
	SolutionName    string                  `json:"solutionName"`
	SupportedDevice string                  `json:"supportedDevice"`
	SamplerMethod   string                  `json:"samplerMethod"`
	LatencyMax      *float64                `json:"latencyMax"`
	TrainDatasets   *RecommendTypedDatasets `json:"trainDatasets"`
	ValDatasets     *RecommendTypedDatasets `json:"valDatasets"`
}

type RecommendTypedDatasets struct {
	Datasets []*RecommendDatasetBody `json:"datasets"`
}

type RecommendDatasetBody struct {
	Total         int                    `json:"total"`
	SdsPath       string                 `json:"sdsPath"`
	Boxes         []*RecommendClassCount `json:"boxes"`
	Classes       []*RecommendClassCount `json:"classes"`
	Keypoints     []*RecommendClassCount `json:"keypoints"`
	Regressors    []*RecommendClassCount `json:"regressors"`
	Segmentations []*RecommendClassCount `json:"segmentations"`
	Conversations []*RecommendClassCount `json:"conversations"`
}

type RecommendClassCount struct {
	Name        string `json:"name"`
	ImageCount  int64  `json:"imageCount"`
	EntityCount int64  `json:"entityCount"`
}

const SnapxText2TextKey = "LLM"
const DefaultTrainDevice = "2080TI"
const SnapxFinetuningKey = "llm_finetuning_type"
const SnapxHyperParamsKey = "hyper_params"
const SnapxSolTypeKey = "sol_type"
