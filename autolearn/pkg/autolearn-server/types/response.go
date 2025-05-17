package types

import (
	aisConst "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
)

type AutoLearnListResp struct {
	Total      int64        `json:"total"`
	AutoLearns []*AutoLearn `json:"autoLearns"`
}

type RevisionListResp struct {
	Total          int64                `json:"total"`
	ExistInvisible bool                 `json:"existInvisible"` // 是否存在隐藏的实验
	Revisions      []*AutoLearnRevision `json:"autoLearnRevisions"`
}

type ExportAlgorithmScoreResp struct {
	AutoLearnName string        `json:"AutoLearnName"`
	RevisionName  string        `json:"revisionName"`
	Train         string        `json:"train"`
	Validation    string        `json:"validation"`
	EvalMetric    *EvalMetric   `json:"evalMetric"`
	Algorithms    []*Algorithm  `json:"algorithms"`
	BestDetector  *BestDetector `json:"bestDetector"`
}

type OptionsResp struct {
	AutoLearnID   string             `json:"autoLearnID" bson:"autoLearnID"`
	AutoLearnName string             `json:"autoLearnName" bson:"autoLearnName"`
	Revisions     []*OptionsRevision `json:"revisions" bson:"autoLearnRevisions"`
}

type OptionsRevision struct {
	RevisionName     string                 `json:"revisionName" bson:"revisionName"`
	RevisionID       string                 `json:"revisionID" bson:"revisionID"`
	Type             aisConst.AppType       `json:"type" bson:"type" enums:"Classification,Detection"` // 场景
	SolutionType     SolutionType           `bson:"solutionType" json:"solutionType"`                  // 当前任务算法类型
	Framework        aisConst.FrameworkType `json:"framework" enums:"PyTorch"`                         // 训练框架
	FrameworkVersion string                 `json:"frameworkVersion"`                                  // 训练框架版本
	ModelType        aisConst.ModelFormat   `json:"modelType" enums:"ONNX,MDL"`                        // 模型类型
	ModelFormat      aisConst.ModelFormat   `json:"modelFormat" enums:"ONNX,MDL"`                      // 模型类型(兼容前端)
	Iterations       []*OptionsIteration    `json:"iterations" bson:"evaluationDetail"`                // 迭代
	IsDeleted        bool                   `bson:"isDeleted" json:"-"`
}

type OptionsIteration struct {
	IterationNumber int32  `json:"iterationNumber" bson:"iterationNumber"` // 迭代次数
	AlgorithmName   string `json:"algorithmName" bson:"algorithmName"`     // 迭代次数
	ModelFilePath   string `json:"modelFilePath" bson:"modelFilePath"`     // 模型文件oss路径
	BestDetector    bool   `json:"bestDetector" bson:"bestDetector"`       // 是否最佳模型
}

type DataSourceAttributeResp struct {
	Attributes    [][]string `json:"attributes"`
	AttributeType string     `json:"attributeType"`
}

type OptimizationResp struct {
	AutoLearnID  string `json:"autoLearnID"`
	RevisionID   string `json:"revisionID"`
	RevisionName string `json:"revisionName"`
}

type EvalComparisonSnapshotResp struct {
	EvalComparisonID     string                `json:"evalComparisonID"` // 自动学习服务的评测对比ID
	Type                 aisConst.AppType      `json:"type"`
	EvalJobs             []*ComparisonEvalJob  `bson:"evalJobs" json:"evalJobs"`
	ComparisonID         string                `json:"comparisonID"`                                     // 来自于评测对比，评测对比ID
	ComparisonSnapshotID string                `json:"comparisonSnapshotID" bson:"comparisonSnapshotID"` // 来自于评测对比，评测对比历史记录ID
	JobIDToJobName       map[string]string     `json:"jobIDToJobName"`                                   // 评测子任务名与评测子任务ID映射
	JobIDToRvName        map[string]string     `json:"jobIDToRvName"`                                    // 评测子任务名与实验名称映射，实验名称为"任务名/实验名/迭代次数"
	RvNameToAlgorithms   map[string]*Algorithm `json:"rvNameToAlgorithms"`                               // 实验名称与算法映射, 实验名称为"任务名/实验名/迭代次数"
}

type EvalComparisonSnapshotListResp struct {
	Total               int64                     `json:"total"`
	ComparisonSnapshots []*EvalComparisonSnapshot `json:"comparisonSnapshots"`
}

type EvalComparisonSnapshotErrorResp struct {
	EvalComparisonID string                    `json:"evalComparisonID"` // 自动学习服务的评测对比ID
	Type             aisConst.AppType          `json:"type"`
	Jobs             []*EvalComparisonErrorJob `json:"Jobs"`
}

type EvalComparisonErrorJob struct {
	AutoLearnID        string `json:"autoLearnID"`
	AutoLearnName      string `json:"autoLearnName"`
	RevisionID         string `json:"revisionID"`
	RevisionName       string `json:"revisionName"`
	IterationNumber    int32  `json:"iterationNumber"`
	EvaluationID       string `json:"evaluationID"` // 评测任务ID
	EvaluationJobID    string `json:"evaluationJobID"`
	EvaluationRJobName string `json:"evaluationRJobName"` // 评测任务RJob名称
}
