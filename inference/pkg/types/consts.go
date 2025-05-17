package types

type ActionType string

const (
	ActionTypeShutdown = "Shutdown"
	ActionTypeRestart  = "Restart"
	ActionTypeRollout  = "Rollout"
)

const (
	MinKeepAliveDuration = 3600 * 1  // 服务自动保持最短时间
	MaxKeepAliveDuration = 3600 * 24 // 服务自动保持最长时间

	S3Schema = "s3"

	ModelStoreURI   = "MODEL_STORE_URI"
	ModelAppType    = "MODEL_APP_TYPE"
	ModelFormatType = "MODEL_FRAMEWORK_TYPE" // todo 修改为Model_FORMAT_TYPE

	SnapSolutionName  = "SNAP_SOLUTION_NAME"
	SnapModelVersion  = "SNAP_MODEL_VERSION"
	SnapModelProtocol = "SNAP_MODEL_PROTOCOL"

	SnapModelVersionV1 = "1"
	SnapModelVersionV2 = "2"

	InferenceServiceNotFound = "not found"

	InitContainerShareVolumeName      = "model-server-location"
	InitContainerShareVolumeMountPath = "/mnt/models"

	InferencePredictorDeploySuffix = "-predictor"

	PodEnvRegex = "[-._a-zA-Z][-._a-zA-Z0-9]*"

	ChatAPIURL = "v1/chat/completions"
)
