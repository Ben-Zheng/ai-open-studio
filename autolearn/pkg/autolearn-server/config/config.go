package config

import (
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/es"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/oss"
	aisTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
)

// Config autoLearn服务配置信息
type Conf struct {
	OssConfig                   *oss.Config               `json:"ossConf" yaml:"ossConf"`
	AisEndPoint                 *AisEndPoint              `json:"aisEndpoint" yaml:"aisEndpoint"`
	AisAccessToken              *AisAccessToken           `json:"aisAccessToken" yaml:"aisAccessToken"`
	GlooGateway                 string                    `json:"glooGateway" yaml:"glooGateway"`
	DetEvalCodebase             *EvalCodebase             `json:"detEvalCodebase" yaml:"detEvalCodebase"`
	ClfEvalCodebase             *EvalCodebase             `json:"clfEvalCodebase" yaml:"clfEvalCodebase"`
	InferCodebase               *EvalCodebase             `json:"inferCodebase" yaml:"inferCodebase"`
	NoriServer                  *NoriServer               `json:"noriServer" yaml:"noriServer"`
	DependentImage              *DependentImage           `json:"dependentImage" yaml:"dependentImage"`
	FluentbitServer             string                    `json:"fluentbitServer" yaml:"fluentbitServer"`
	ESConfig                    *es.Config                `json:"esConfig" yaml:"esConfig"`
	SnapxResouceMode            string                    `json:"snapxResouceMode" yaml:"snapxResouceMode" enums:"internal,external"`
	UseOldModelZipPath          bool                      `json:"useOldModelZipPath" yaml:"useOldModelZipPath"`
	MultiSiteConfig             *aisTypes.MultiSiteConfig `json:"multiSiteConfig" yaml:"multiSiteConfig"`
	GPUOption                   *GPUOption                `json:"GPUOption" yaml:"GPUOption"`
	AutoLearnDetectionF1Enabled bool                      `json:"autoLearnDetectionF1Enabled" yaml:"autoLearnDetectionF1Enabled"`
}

// AisEndPoint ais中其他服务的地址
type AisEndPoint struct {
	DatahubAPIServer string `json:"datahubAPIServer" yaml:"datahubAPIServer"`
	DatahubRelabel   string `json:"datahubRelabel" yaml:"datahubRelabel"`
	EvalHub          string `json:"evalHub" yaml:"evalHub"`
	AutoLearn        string `json:"autoLearn" yaml:"autoLearn"`
	AutoLearnSampler string `json:"autoLearnSampler" yaml:"autoLearnSampler"`
	SolutionHub      string `json:"solutionHub" yaml:"solutionHub"`
	AuthServer       string `json:"authServer" yaml:"authServer"`
	PublicServer     string `json:"publicServer" yaml:"publicServer"`
	Codehub          string `json:"codehub" yaml:"codehub"`
	Resourcesense    string `json:"resourcesense" yaml:"resourcesense"`
}

// 用于auth验证的ais服务的Ak/Sk
type AisAccessToken struct {
	AccessKey string `json:"accessKey" yaml:"accessKey"`
	SecretKey string `json:"secretKey" yaml:"secretKey"`
}

// 用于建立评测任务的codebase及job配置
type EvalCodebase struct {
	CodeID       string `json:"codeID" yaml:"codeID"`
	CodeName     string `json:"codeName" yaml:"codeName"`
	CPUPerJob    int32  `json:"cpuPerJob" yaml:"cpuPerJob"`
	GPUPerJob    int32  `json:"gpuPerJob" yaml:"gpuPerJob"`
	MemoryPerJob int32  `json:"memoryPerJob" yaml:"memoryPerJob"`
}

type NoriServer struct {
	ControllerAddr     string            `json:"controllerAddr" yaml:"controllerAddr"`
	LocateAddr         string            `json:"locateAddr" yaml:"locateAddr"`
	OSSProxyAddr       string            `json:"ossProxyAddr" yaml:"ossProxyAddr"`
	StateQueryInterval int16             `json:"stateQueryInterval" yaml:"stateQueryInterval"`
	StateQueryMaxRetry int16             `json:"stateQueryMaxRetry" yaml:"stateQueryMaxRetry"`
	NoriEnv            map[string]string `json:"noriEnv" yaml:"noriEnv"`
}

type DependentImage struct {
	Snapx           string `json:"snapx" yaml:"snapx"`
	SnapWorkerAgent string `json:"snapWorkerAgent" yaml:"snapWorkerAgent"`
	InternalAgent   string `json:"internalAgent" yaml:"internalAgent"`
}

type GPUOption struct {
	TrainAlias []string `json:"trainAlias" yaml:"trainAlias"`
	InferAlias []string `json:"inferAlias" yaml:"inferAlias"`
}
