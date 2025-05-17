package types

import (
	v1 "k8s.io/api/core/v1"

	modeltypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/modelhub/pkg/types"
	aisTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/utils/sds"
)

type CreateInferenceServiceReq struct {
	Name        string                       `json:"name"`
	Description string                       `json:"description"`
	Tags        []string                     `json:"tags"`
	AppType     aisTypes.AppType             `json:"appType"`
	Approach    aisTypes.Approach            `json:"approach,omitempty"`
	Revision    *CreateInferenceServiceRvReq `json:"revision"`
}

type CreateInferenceServiceRvReq struct {
	ServiceInstances []*CreateServiceInstanceReq `json:"serviceInstances"` // 更新版本或创建第一个版本时使用
}

type CreateServiceInstanceReq struct {
	From              InferenceInstanceFrom      `json:"from"`
	ImageURI          string                     `json:"imageURI"`
	ModelID           string                     `json:"modelID"`
	ModelRevision     string                     `json:"modelRevision"`
	ModelAppType      aisTypes.AppType           `json:"modelAppType"`
	ModelFormatType   modeltypes.ModelFormatType `json:"modelFormatType"`
	ModelURI          string                     `json:"modelURI"`
	EntryPoint        []string                   `json:"entryPoint"`
	Env               []v1.EnvVar                `json:"env"`
	MinReplicas       int                        `json:"minReplicas"`
	MaxReplicas       int                        `json:"maxReplicas"`
	Resource          *Resource                  `json:"resource"`
	AutoShutdown      bool                       `json:"autoShutdown"`
	KeepAliveDuration int                        `json:"keepAliveDuration"` // 单位s
	Device            string                     `json:"device"`
	Platform          string                     `json:"platform"`
	PlatformDevice    string                     `json:"platformDevice"`

	ModelParentRevision string `json:"-"` // 从模型服务获取
}

type RevisionActionRequest struct {
	Action          ActionType `json:"action" enums:"Shutdown, Restart, Rollout"`
	RolloutRevision string     `json:"rolloutRevision"`
}

type PatchInferenceServiceReq struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
}

type UpdateInferenceServiceReason struct {
	Reason string `json:"reason"`
}

type PatchIsvcRevisionReq struct {
	Hyperparameter *Hyperparameter `json:"hyperparameter,omitempty" bson:"hyperparameter,omitempty"`
}

type CreateConversationRecordReq struct {
	Name string `json:"name,omitempty" bson:"name,omitempty"`
}

type PatchConversationRecordReq struct {
	Name string `json:"name,omitempty" bson:"name,omitempty"`
}

type OpenAIChatReq struct {
	Model            string         `json:"model"`
	Messages         []*ChatMessage `json:"messages" validate:"required"`
	FrequencyPenalty float64        `json:"frequency_penalty"` // 重复惩罚
	Temperature      float64        `json:"temperature"`
	TopP             float64        `json:"top_p"`
	Stream           bool           `json:"stream"`
}

type ChatMessage struct {
	Role    sds.Role `json:"role" validate:"required"`
	Content any      `json:"content" validate:"required"`
}

type ChatMessageContentForTextAndImage struct {
	Type     string `json:"type" validate:"required" enums:"text,image_url"`
	Text     string `json:"text"`
	ImageURL struct {
		URL string `json:"url"`
	} `json:"image_url"` // 目前仅支持base64编码
}
