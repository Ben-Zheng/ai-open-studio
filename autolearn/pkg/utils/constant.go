package utils

const (
	SN_OUTPUT_DIR             = "/data/snapdet/output_dir"
	ONNX_DIR                  = "/data/snapdet/output"
	MODEL_BOX_DETECTOR        = "model.box-detector.zip"
	MODEL_DOWNLOAD_NAME       = "model.zip"
	DET_TRAINED_MODEL_NAME    = "input.snapdet-box-detector.predictor"
	CLF_TRAINED_MODEL_NAME    = "input.snapclf-classifier.predictor"
	BEST_DETECTOR_PATH        = "best_detector.snapdet-box-detector"
	BEST_CLASSIFIER_PATH      = "best_classifier.snapclf-classifier"
	RECOMMEND_SOLUTION_FILE   = "/data/snapdet/recommend.solution-config"
	CONTINUED_LEARN_MODEL_DIR = "/data/snapdet/continued-model"
	SNAP_SAMPLER_FILE         = "/data/snapdet/sampler.json"
	DETECTOR_MODEL_ZIP_PATH   = "/data/snapdet/output_dir/detectors/%s.snapdet-box-detector.inference-model.zip"
	DETECTOR_PATH             = "/data/snapdet/output_dir/detectors/%s.snapdet-box-detector"
	CLASSIFIER_MODEL_ZIP_PATH = "/data/snapdet/output_dir/classifiers/%s.snapclf-classifier_inference-model.zip"
	CLASSIFIER_PATH           = "/data/snapdet/output_dir/classifiers/%s.snapclf-classifier"
	AUTOLEARN_S3_PREFIX       = "s3://system-%s-autolearn/autolearn/id-%s/revision-%s"
	EARY_STOP_FILE            = "early_stop.txt"
	PREDICT_SDS               = "val.pred.sds"
	PREDICT_SDS_INDEX         = "1.sds"
	PREDICT_META_JSON         = "meta.json"
	DEFAUL_FRAMEWORK          = "Pytorch"
	DEFAULT_APPTYPE           = "Detection"
	PREDICT_FOLDER_SUFFIX     = ".pred"
	AUTOLEARN_APISERVER_ADDR  = "APISERVER_ADDR"
	POD_TYPE                  = "POD_TYPE"
	POD_NAME                  = "POD_NAME"
	WORKER_NUM                = "WORKER_NUM"
	AUTOLEARN_NAME            = "AUTOLEARN_NAME"
	OSS_ACCESSKEY             = "OSS_ACCESSKEY_ID"
	OSS_ACCESSKEY_SECRET      = "OSS_ACCESSKEY_SECRET"
	OSS_ENDPOINT              = "OSS_ENDPOINT"
	SNAPDET_TRAIN_MODE        = "SNAPDET_TRAIN_MODE"
	TIME_LIMIT                = "TIME_LIMIT"
	RECALL_ZERO_VALUE         = -1
	TRAIN_CODE_FILE           = "/data/snapdet/output_dir/status"

	BEST_CLASSIFIER_HISTORY = "best_classifier_history.json"
	META_JSON_CLF           = "classifiers.meta.jsonl"
	BEST_DETECTOR_HISTROY   = "best_detector_history.json"
	MEAT_JSON_DET           = "detectors.meta.jsonl"

	// 产出物后缀名，修改需谨慎
	// 新数据后缀
	ONNX_MODEL_SUFFIX         = "model.onnx.zip"
	BOX_DETECTOR_MODEL_SUFFIX = "model.box-detector.zip"
	MDL_MODEL_SUFFIX          = "model.mdl.zip"
	// 旧数据后缀
	MODEL_ZIP = "/model.zip"

	CONVERTOR_INPUT_SIZE_ENV   = "AUTOLEARN_INPUT_SIZE"
	CONVERTOR_FILTER_MODE_ENV  = "AUTOLEARN_FILTER_MODE"
	CONVERTOR_INPUT_FORMAT_ENV = "AUTOLEARN_INPUT_FORMAT"
	CONVERTOR_ITER_ENV         = "AUTOLEARN_ITERATOR_NUM"
	CONVERTOR_ID_ENV           = "AUTOLEARN_CONVERTOR_ID"
	SOURCE_MODEL_ENV           = "SOURCE_MODEL_PATH"
	TARGET_MODEL_ENV           = "TARGET_MODEL_PATH"

	DETECTOR_MODEL_ZIP_PATH_ENV   = "DETECTOR_MODEL_ZIP_PATH"
	CLASSIFIER_MODEL_ZIP_PATH_ENV = "CLASSIFIER_MODEL_ZIP_PATH"

	AutolearnID       = "AUTOLEARN_ID"
	AutolearnRevision = "AUTOLEARN_REVISION"
	AutolearnTenant   = "AUTOLEARN_TENANT"
	AutolearnProject  = "AUTOLEARN_PROJECT"
)
