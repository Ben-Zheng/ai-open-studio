package types

import (
	"go.mongodb.org/mongo-driver/bson/primitive"
	v1 "k8s.io/api/core/v1"

	modeltypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/modelhub/pkg/types"
	aistypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/utils/sds"
)

type InferenceService struct {
	ID                  string                      `json:"ID" bson:"ID"`
	ProjectID           string                      `json:"projectID" bson:"projectID"`
	Name                string                      `json:"name" bson:"name"`
	AppType             aistypes.AppType            `json:"appType" bson:"appType"`
	Approach            aistypes.Approach           `bson:"approach,omitempty" json:"approach,omitempty"`
	Description         string                      `json:"description" bson:"description"`
	Tags                []string                    `json:"tags" bson:"tags"`
	ServiceURI          string                      `json:"serviceURI" bson:"serviceURI"`
	ServiceInternalURI  string                      `json:"-" bson:"serviceInternalURI"`
	CreatedBy           string                      `json:"createdBy" bson:"createdBy"`
	CreatedAt           int64                       `json:"createdAt" bson:"createdAt"`
	UpdatedBy           string                      `json:"updatedBy" bson:"updatedBy"`
	UpdatedAt           int64                       `json:"updatedAt" bson:"updatedAt"`
	RevisionCount       int64                       `json:"revisionCount" bson:"revisionCount"`
	Revisions           []*InferenceServiceRevision `json:"revisions" bson:"revisions"`
	InstancePods        []*InstancePod              `json:"instancePods" bson:"instancePods"`
	RequestTotal        int64                       `json:"requestTotal" bson:"requestTotal"`
	RequestSuccessCount int64                       `json:"requestSuccessCount" bson:"requestSuccessCount"`
	IsDeleted           bool                        `json:"-" bson:"isDeleted"`
}

type InferenceServiceState = string

const (
	InferenceServiceStateUnknown      InferenceServiceState = "Unknown"
	InferenceServiceStateDeploying    InferenceServiceState = "Deploying"
	InferenceServiceStateDeployFailed InferenceServiceState = "DeployFailed"
	InferenceServiceStateNormal       InferenceServiceState = "Normal"
	InferenceServiceStateAbnormal     InferenceServiceState = "Abnormal"
	InferenceServiceStateShutdown     InferenceServiceState = "Shutdown"
)

type InferenceNotReadyReason string

const (
	InferenceNotReadyReasonUnknown  InferenceNotReadyReason = "Unknown"
	InferenceNotReadyReasonStarting InferenceNotReadyReason = "Starting"
	InferenceNotReadyReasonFailed   InferenceNotReadyReason = "Failed"
)

type InferenceServiceRevisionStage string

const (
	InferenceServiceRevisionStageShutdown     InferenceServiceRevisionStage = "Shutdown"
	InferenceServiceRevisionStageDeploying    InferenceServiceRevisionStage = "Deploy"
	InferenceServiceRevisionStageServing      InferenceServiceRevisionStage = "Serve"
	InferenceServiceRevisionStageDeployFailed InferenceServiceRevisionStage = "DeployFailed"
)

type FailedReason string

const (
	FailedReasonScheduleFailed = "PodScheduleFailed" // 资源调度失败，可能当前资源不足，请查看资源是否充足并重建或咨询客服
	FailedReasonInitFailed     = "InitFailed"        // 任务初始化失败，请重试
	FailedReasonStartFailed    = "StartFailed"       // 推理工具启动失败，请重试或咨询客服
)

var FailedReasonMap = map[FailedReason]string{
	"":                         "资源调度失败，可能当前资源不足，请查看资源是否充足并重建或咨询客服",
	FailedReasonScheduleFailed: "资源调度失败，可能当前资源不足，请查看资源是否充足并重建或咨询客服",
	FailedReasonInitFailed:     "任务初始化失败，请重试",
	FailedReasonStartFailed:    "推理工具启动失败，请重试或咨询客服",
}

type InferenceServiceRevision struct {
	Revision         string                        `json:"revision" bson:"revision"`
	ParentRevision   string                        `json:"parentRevision" bson:"parentRevision"`
	Ready            bool                          `json:"ready" bson:"ready"`
	Canary           bool                          `json:"canary" bson:"canary"`
	Active           bool                          `json:"active" bson:"active"` // 决定哪个revision是当前活动的
	ServiceInstances []*InferenceInstance          `json:"serviceInstances" bson:"serviceInstances"`
	CreatedAt        int64                         `json:"createdAt" bson:"createdAt"`
	CreatedBy        string                        `json:"createdBy" bson:"createdBy"`
	QuotaConfig      QuotaConfig                   `json:"-" bson:"quotaConfig"`
	Stage            InferenceServiceRevisionStage `json:"stage" bson:"stage"`
	State            InferenceServiceState         `json:"state" bson:"-"`
	NotReadyReason   string                        `json:"notReadyReason" bson:"-"`
	FailedReason     string                        `json:"failedReason" bson:"-"`
	Hyperparameter   *Hyperparameter               `json:"hyperparameter,omitempty" bson:"hyperparameter,omitempty"`

	ConversationRecords []*ConversationRecord `json:"conversationRecords,omitempty" bson:"-"`
}

type ConversationRecord struct {
	ID                 primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	InferenceServiceID string             `json:"inferenceServiceID,omitempty" bson:"inferenceServiceID,omitempty"`
	IsvcRevisionID     string             `json:"isvcRevisionID,omitempty" bson:"isvcRevisionID,omitempty"`
	Name               string             `json:"name,omitempty" bson:"name,omitempty"`
	Conversations      []*Conversation    `json:"conversations" bson:"conversations"`
	CreatedAt          int64              `json:"createdAt" bson:"createdAt"`
	TotalTokens        int64              `json:"-" bson:"totalTokens"`
	TobeMaxTokens      bool               `json:"tobeMaxTokens" bson:"-"`
	IsDeleted          bool               `json:"-" bson:"isDeleted"`
}

type Conversation struct {
	ID       primitive.ObjectID `json:"id" bson:"id"`
	Role     sds.Role           `json:"role,omitempty" bson:"role,omitempty"`
	Modality sds.Modality       `json:"modality,omitempty" bson:"modality,omitempty"`
	Content  string             `json:"content,omitempty" bson:"content,omitempty"`
}

type Hyperparameter struct {
	SystemPersonality string  `json:"systemPersonality,omitempty" bson:"systemPersonality,omitempty"`
	Temperature       float64 `json:"temperature,omitempty" bson:"temperature,omitempty"` //
	TopP              float64 `json:"topP,omitempty" bson:"topP,omitempty"`
	PenaltyScore      float64 `json:"penaltyScore,omitempty" bson:"penaltyScore,omitempty"`
}

type InferenceInstanceFrom string

const (
	InferenceInstanceFromModel InferenceInstanceFrom = "Model"
	InferenceInstanceFromImage InferenceInstanceFrom = "Image"
)

// InferenceInstance define an instance for InferenceService
type InferenceInstance struct {
	ID     string                   `json:"ID" bson:"ID"`
	Meta   *InferenceInstanceMeta   `json:"meta" bson:"meta"`
	Spec   *InferenceInstanceSpec   `json:"spec" bson:"spec"`
	Status *InferenceInstanceStatus `json:"status" bson:"status"`
}

type InferenceInstanceMeta struct {
	From                    InferenceInstanceFrom      `json:"from" bson:"from"`
	ImageURI                string                     `json:"imageURI" bson:"imageURI"`
	ModelID                 string                     `json:"modelID" bson:"modelID"`
	ModelRevision           string                     `json:"modelRevision" bson:"modelRevision"`
	ModelParentRevision     string                     `json:"modelParentRevision" bson:"modelParentRevision"`
	Platform                string                     `json:"platform" bson:"platform"`
	Device                  string                     `json:"device" bson:"device"`
	PlatformDevice          string                     `json:"platformDevice" bson:"platformDevice"`
	ModelAppType            string                     `json:"modelAppType" bson:"modelAppType"`
	ModelFormatType         modeltypes.ModelFormatType `json:"modelFormat" bson:"modelFormatType"`
	ModelURI                string                     `json:"modelURI" bson:"modelURI"`
	EntryPoint              []string                   `json:"entryPoint" bson:"entryPoint"`
	Env                     []v1.EnvVar                `json:"env" bson:"env"`
	GPUTypes                []string                   `json:"gpuTypes" bson:"gpuTypes"`
	ModelName               string                     `json:"modelName" bson:"-"`
	ModelRevisionName       string                     `json:"modelRevisionName" bson:"-"`
	ModelParentRevisionName string                     `json:"modelParentRevisionName" bson:"-"`
	ModelConvertName        string                     `json:"modelConvertName" bson:"-"`

	SnapModelVersion  string `json:"-" bson:"snapModelVersion"`
	SnapSolutionName  string `json:"-" bson:"snapSolutionName"`
	SnapModelProtocol string `json:"-" bson:"snapModelProtocol"`
}

type InferenceInstanceSpec struct {
	MinReplicas    int      `json:"minReplicas" bson:"minReplicas"`
	MaxReplicas    int      `json:"maxReplicas" bson:"maxReplicas"`
	Concurrent     int      `json:"concurrent" bson:"concurrent"`
	TrafficPercent int      `json:"trafficPercent" bson:"trafficPercent"`
	Resource       Resource `json:"resource" bson:"resource"`

	AutoShutdown      bool `json:"autoShutdown" bson:"autoShutdown"`
	KeepAliveDuration int  `json:"keepAliveDuration" bson:"keepAliveDuration"` // 单位s
}

type InferenceInstanceStatus struct {
	Ready          bool         `json:"ready" bson:"ready"`
	NotReadyReason string       `json:"notReadyReason" bson:"notReadyReason"`
	FailedReason   FailedReason `json:"failedReason" bson:"failedReason"`
	InstanceURI    string       `json:"instanceURI" bson:"instanceURI"`
	Pods           []string     `json:"pods" bson:"-"`
	UpdatedAt      int64        `json:"updatedAt" bson:"updatedAt"`
}

type QuotaConfig struct {
	QuotaGroup  string `json:"quotaGroup" bson:"quotaGroup"`
	BestEffort  string `json:"bestEffort" bson:"bestEffort"`
	Preemptible string `json:"preemptible" bson:"preemptible"`
}

type Resource struct {
	GPU     int32  `json:"gpu" bson:"gpu"`
	CPU     int32  `json:"cpu" bson:"cpu"`
	Memory  int32  `json:"memory" bson:"memory"`
	GPUType string `json:"gpuType,omitempty" bson:"gpuType,omitempty"`
}

type InferenceRequestLog struct {
	ID                 primitive.ObjectID `json:"id" bson:"_id"`
	InferenceServiceID string             `json:"inferenceServiceID" bson:"inferenceServiceID"`
	CreatedAt          int64              `json:"createdAt" bson:"createdAt"`
	Source             string             `json:"source" bson:"source"`
	Revision           string             `json:"revision" bson:"revision"`
	InstanceID         string             `json:"instanceID" bson:"instanceID"`
	APIURL             string             `json:"apiUrl" bson:"apiUrl"`
	Response           string             `json:"response" bson:"response"`
	Status             int                `json:"status" bson:"status"`
}

type InstancePod struct {
	Name      string `json:"name" bson:"name"`
	CreatedAt int64  `json:"createdAt" bson:"createdAt"`
	Type      string `json:"type" bson:"type"`
}
