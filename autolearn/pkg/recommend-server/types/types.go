package types

const (
	QUERY_TYPE           = "type"
	QUERY_SOL_NUM        = "sol"
	QUERY_TIME_LIMIT     = "timeLimit"
	QUERY_TRAIN_SDS      = "trainSDS"
	QUERY_VAL_SDS        = "valSDS"
	QUERY_TRAIN_TYPE     = "type"
	QUERY_LATENCY_MAX    = "latencyMax"
	QUERY_LATENCY_DEVICE = "latencyDevice"
	QUERY_LATENCY_INPUT  = "latencyInput"
	QUERY_MODEL_TYPE     = "modelType"
	QUERY_SOLUTION_NAME  = "solutionName"

	DEFAULT_DIR_LEN            = 8
	DEFAULT_MODEL_TYPE         = "onnx"
	DEFAULT_MODEL_TM_OTHERS    = "tm_others" // deprecated
	DEFAULT_MODEL_TM_MC40      = "tm_mc40"
	DEFAULT_MODEL_MC40         = "mc40" // deprecated
	DEFAULT_MODEL_TM_TRT       = "tm_trt"
	DEFAULT_MODEL_TM_RKNN      = "tm_rknn"
	DEFAULT_MODEL_TM_MC20      = "tm_mc20"
	DEFAULT_MODEL_TM_SIGMASTAR = "tm_sigmastar"
	DEFAULT_MODEL_TM_T40       = "tm_t40"
	DEFAULT_MODEL_TM_CVITEK    = "tm_cvitek"
	DEFAULT_MODEL_TM_NNIE      = "tm_nnie"

	DEFAULT_RECOMMEND_TIMEOUT_SECONDS = "150"
	RECOMMEND_CODE_FILE               = "status"
	DEFAULT_SOLUTION_FILE_NAME        = "recommend.solution-config"

	// snapdet property keys
	PROPERTY_MACS_KEY             = "MACs"
	PROPERTY_LATENCY_KEY          = "latency"
	PROPERTY_MODEL_SIZE_KEY       = "model_size"
	PROPERTY_DEFAULT_LATENCY_UNIT = "ms"
	PROPERTY_DEFAULT_MAC_UNIT     = "G"
	PROPERTY_DEFAULT_SIZE_UNIT    = "MB"

	SOLUTION_TYPE_CLF     = "classification"
	SOLTUION_TYPE_REG     = "regression"
	SOLTUION_TYPE_CLF_REG = "classification+regression"
)

type Config struct {
	GinConf struct {
		ServerAddr string `json:"serverAddr" yaml:"serverAddr"`
	} `json:"ginConf" yaml:"ginConf"`
}

type CategorySummarize struct {
	ClassNames []string `json:"class_names"`
}
