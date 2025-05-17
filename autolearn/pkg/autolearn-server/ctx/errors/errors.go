package errors

import "net/http"

// ErrorCode 是前后端约定的,用来标识需要处理case的那些错误码
type ErrorCode string

var (
	InternalError                        ErrorCode = "internal.error"
	ParamsInvalid                        ErrorCode = "params.invalid"
	OperationInvalid                     ErrorCode = "operation.invalid"
	AutoLearnDuplicate                   ErrorCode = "autolearn.duplicate"
	RevisionDuplicate                    ErrorCode = "revision.duplicate"
	AutoLearnNameInvalid                 ErrorCode = "autolearn.name.invalid"
	RevisionNameInvalid                  ErrorCode = "revision.name.invalid"
	RevisionIDDuplicate                  ErrorCode = "revisionID.duplicate"
	AutoLearnNotFound                    ErrorCode = "autolearn.not.found"
	AutoLearnRevisionNotFound            ErrorCode = "autolearn.revision.not.found"
	QuotaGroupInsufficientQuota          ErrorCode = "quota.group.not.sufficient"
	ProjectQuotaInsufficientQuota        ErrorCode = "project.quota.not.sufficient"
	ObjectNotFound                       ErrorCode = "object.not.found"
	StateMachineNotSupported             ErrorCode = "state.machine.not.supported"
	ModelNotFound                        ErrorCode = "model.file.not.found"
	EvalHubServiceUnavailable            ErrorCode = "eval.service.unavailable"
	EvaluationJobExist                   ErrorCode = "eval.job.exist"
	EvaluationJobNotNotFound             ErrorCode = "eval.job.not.found"
	EvaluationJobNotNotUnArchived        ErrorCode = "eval.job.not.unarchived"
	EvaluationNotNotFound                ErrorCode = "eval.not.found"
	InternalDatasetInvalid               ErrorCode = "internal.dataset.invalid"
	DatasetNotContainClass               ErrorCode = "dataset.not.contain.class"
	DatasetClassessNotMatch              ErrorCode = "dataset.classes.not.match"
	APUTypeNotMatch                      ErrorCode = "apu.type.not.match"
	InternalDatasetService               ErrorCode = "internal.datahub.service.unavailable"
	DatasetSourceTypeInvalid             ErrorCode = "dataset.source.type.not.supported"
	GetAlgorithmFailed                   ErrorCode = "get.algorithm.failed"
	GetAlgorithmFailedByInvalidSDS       ErrorCode = "get.algorithm.failed.by.invalid.sds"
	GetAlgorithmFailedByTimeout          ErrorCode = "get.algorithm.failed.by.timeout"
	GetAlgorithmFailedByNotFound         ErrorCode = "get.algorithm.failed.by.notfound"
	GetAlgorithmFailedByNotFulfill       ErrorCode = "get.algorithm.failed.by.notfulfill"
	BuildSnapSampler                     ErrorCode = "build.snap.sampler"
	UpdateSnapSampler                    ErrorCode = "update.snap.sampler"
	GetSnapSamplerFailedByTimeout        ErrorCode = "get.snap.sampler.failed.by.timeout"
	GetClassFailed                       ErrorCode = "get.class.failed"
	DatasetNotFound                      ErrorCode = "dataset.not.found"
	SDSFileNotFound                      ErrorCode = "sds.not.found"
	ParseSDSFileFailed                   ErrorCode = "parse.sds.failed"
	InconstantDatasetType                ErrorCode = "inconstant.dataset.type"
	BoxDetectorNotFound                  ErrorCode = "box-detector.not.exist"
	EvalComparisonNotFound               ErrorCode = "eval.comparison.not.found"
	EvalComparisonSnapshotNotFound       ErrorCode = "eval.comparison.snapshot.not.found"
	NoSuccessfulEvalJob                  ErrorCode = "no.successful.eval.job.for.revision"
	EvalComparisonJobAlreadyArchived     ErrorCode = "eval.comparison.job.already.archived"
	AutoLearnAPIUnavailable              ErrorCode = "autolearn.api.service.unavailable"
	AutoLearnRebuildAlgorithmUnavailable ErrorCode = "autolearn.rebuild.algorithm.unavailable"
)

type AutoLearnError struct {
	httpCode  int
	errorCode ErrorCode
	message   string
}

func (err AutoLearnError) Error() string {
	return err.message
}

func (err AutoLearnError) HTTPCode() int {
	return err.httpCode
}

func (err AutoLearnError) ErrorCode() ErrorCode {
	return err.errorCode
}

func (err AutoLearnError) Message() string {
	return err.message
}

func (err *AutoLearnError) SetMessage(msg string) *AutoLearnError {
	err.message = msg
	return err
}

var (
	ErrorInternal = &AutoLearnError{
		httpCode:  http.StatusInternalServerError,
		errorCode: InternalError,
		message:   "internal server error",
	}

	ErrorInvalidS3URI = &AutoLearnError{
		httpCode:  http.StatusBadRequest,
		errorCode: ParamsInvalid,
		message:   "s3 uri format is invalid",
	}

	ErrorDuplicateName = &AutoLearnError{
		httpCode:  http.StatusBadRequest,
		errorCode: AutoLearnDuplicate,
		message:   "duplicate name",
	}

	ErrorDuplicateRevisionName = &AutoLearnError{
		httpCode:  http.StatusBadRequest,
		errorCode: RevisionDuplicate,
		message:   "duplicate name",
	}

	ErrorDuplicateRevisionID = &AutoLearnError{
		httpCode:  http.StatusInternalServerError,
		errorCode: RevisionIDDuplicate,
		message:   "duplicate revisionID",
	}

	ErrorInvalidParams = &AutoLearnError{
		httpCode:  http.StatusBadRequest,
		errorCode: ParamsInvalid,
		message:   "invalid params",
	}

	ErrorInvalidOperation = &AutoLearnError{
		httpCode:  http.StatusBadRequest,
		errorCode: OperationInvalid,
		message:   "invalid Operation",
	}

	ErrorAutoLearnNotFound = &AutoLearnError{
		httpCode:  http.StatusNotFound,
		errorCode: AutoLearnNotFound,
		message:   "autoLearn not found",
	}

	ErrorAutoLearnRvNotFound = &AutoLearnError{
		httpCode:  http.StatusNotFound,
		errorCode: AutoLearnRevisionNotFound,
		message:   "autoLearn revision not found",
	}

	ErrorModelNotFound = &AutoLearnError{
		httpCode:  http.StatusNotFound,
		errorCode: ModelNotFound,
		message:   "model file not found",
	}

	ErrorObjectNotFound = &AutoLearnError{
		httpCode:  http.StatusBadRequest,
		errorCode: ObjectNotFound,
		message:   "object not found",
	}

	ErrorNameRegexp = &AutoLearnError{
		httpCode:  http.StatusBadRequest,
		errorCode: AutoLearnNameInvalid,
		message:   "invalid name",
	}

	ErrorInsufficientQuota = &AutoLearnError{
		httpCode:  http.StatusBadRequest,
		errorCode: QuotaGroupInsufficientQuota,
		message:   "insufficient quota",
	}

	ErrorInsufficientProjectQuota = &AutoLearnError{
		httpCode:  http.StatusBadRequest,
		errorCode: ProjectQuotaInsufficientQuota,
		message:   "insufficient quota",
	}

	ErrorStateMachineChange = &AutoLearnError{
		httpCode:  http.StatusBadRequest,
		errorCode: StateMachineNotSupported,
		message:   "State machine not supported",
	}

	ErrorEvalHubService = &AutoLearnError{
		httpCode:  http.StatusInternalServerError,
		errorCode: EvalHubServiceUnavailable,
		message:   "evalHub service call exception",
	}

	ErrorEvalJobNotUnArchived = &AutoLearnError{
		httpCode:  http.StatusBadRequest,
		errorCode: EvaluationJobNotNotUnArchived,
		message:   "evaluation job not unarchived",
	}

	ErrorEvaluation = &AutoLearnError{
		httpCode:  http.StatusBadRequest,
		errorCode: EvaluationJobExist,
		message:   "evaluation job already exist",
	}

	ErrorEvalJobNotFound = &AutoLearnError{
		httpCode:  http.StatusNotFound,
		errorCode: EvaluationJobNotNotFound,
		message:   "evaluation job not exist",
	}

	ErrorEvaluationNotFound = &AutoLearnError{
		httpCode:  http.StatusNotFound,
		errorCode: EvaluationNotNotFound,
		message:   "evaluation job not exist",
	}

	ErrorDatasetNotFound = &AutoLearnError{
		httpCode:  http.StatusNotFound,
		errorCode: DatasetNotFound,
		message:   "dataset not found",
	}

	ErrorDataHubService = &AutoLearnError{
		httpCode:  http.StatusBadRequest,
		errorCode: InternalDatasetService,
		message:   "dataset service is not available",
	}

	ErrorInvalidDataset = &AutoLearnError{
		httpCode:  http.StatusBadRequest,
		errorCode: InternalDatasetInvalid,
		message:   "dataset not published or sds URI is null",
	}

	ErrorNotClassificationDataset = &AutoLearnError{
		httpCode:  http.StatusBadRequest,
		errorCode: DatasetNotContainClass,
		message:   "not classification dataset",
	}

	ErrorDatasetClassesNotMatch = &AutoLearnError{
		httpCode:  http.StatusBadRequest,
		errorCode: DatasetClassessNotMatch,
		message:   "dataset classes is not matched",
	}

	ErrorFileType = &AutoLearnError{
		httpCode:  http.StatusBadRequest,
		errorCode: DatasetSourceTypeInvalid,
		message:   "only support file with sds extension",
	}

	ErrorGetAlgorithmFailed = &AutoLearnError{
		httpCode:  http.StatusInternalServerError,
		errorCode: GetAlgorithmFailed,
		message:   "failed to get algorithm",
	}

	ErrorGetAlgorithmFailedByInvalidSDS = &AutoLearnError{
		httpCode:  http.StatusInternalServerError,
		errorCode: GetAlgorithmFailedByInvalidSDS,
		message:   "failed to get algorithm cause by invaild SDS",
	}

	ErrorGetAlgorithmFailedByTimeout = &AutoLearnError{
		httpCode:  http.StatusInternalServerError,
		errorCode: GetAlgorithmFailedByTimeout,
		message:   "failed to get algorithm cause by timeout",
	}

	ErrorGetAlgorithmFailedByNotFound = &AutoLearnError{
		httpCode:  http.StatusInternalServerError,
		errorCode: GetAlgorithmFailedByNotFound,
		message:   "failed to get algorithm cause by not found",
	}

	ErrorGetAlgorithmFailedByNotFulfill = &AutoLearnError{
		httpCode:  http.StatusInternalServerError,
		errorCode: GetAlgorithmFailedByNotFulfill,
		message:   "failed to get algorithm cause by not fulfill required number",
	}

	ErrorBuildSnapSampler = &AutoLearnError{
		httpCode:  http.StatusInternalServerError,
		errorCode: BuildSnapSampler,
		message:   "failed to build snap sampler",
	}

	ErrorUpdateSnapSampler = &AutoLearnError{
		httpCode:  http.StatusInternalServerError,
		errorCode: UpdateSnapSampler,
		message:   "failed to update snap sampler",
	}

	ErrorGetSnapSamplerFailedByTimeout = &AutoLearnError{
		httpCode:  http.StatusInternalServerError,
		errorCode: GetSnapSamplerFailedByTimeout,
		message:   "failed to get snap sampler cause by timeout",
	}

	ErrorGetClassFailed = &AutoLearnError{
		httpCode:  http.StatusInternalServerError,
		errorCode: GetClassFailed,
		message:   "failed to get class",
	}

	ErrorSDSNotFound = &AutoLearnError{
		httpCode:  http.StatusNotFound,
		errorCode: SDSFileNotFound,
		message:   "sds file not found",
	}

	ErrorParseSDDFailed = &AutoLearnError{
		httpCode:  http.StatusBadRequest,
		errorCode: ParseSDSFileFailed,
		message:   "parse sds file failed",
	}

	ErrorAPUTypeNotMatch = &AutoLearnError{
		httpCode:  http.StatusBadRequest,
		errorCode: APUTypeNotMatch,
		message:   "apu type not match",
	}

	ErrorInconsistentDatasetType = &AutoLearnError{
		httpCode:  http.StatusBadRequest,
		errorCode: InconstantDatasetType,
		message:   "inconsistent dataset type",
	}

	ErrorBoxDetectorNotFound = &AutoLearnError{
		httpCode:  http.StatusBadRequest,
		errorCode: BoxDetectorNotFound,
		message:   "box-detector file not found",
	}

	ErrorEvalComparisonNotFound = &AutoLearnError{
		httpCode:  http.StatusBadRequest,
		errorCode: EvalComparisonNotFound,
		message:   "eval comparison not found",
	}

	ErrorEvalComparisonSnapshotNotFound = &AutoLearnError{
		httpCode:  http.StatusNotFound,
		errorCode: EvalComparisonSnapshotNotFound,
		message:   "eval comparison snapshot not found",
	}

	ErrorNoSuccessfulEvalJob = &AutoLearnError{
		httpCode:  http.StatusBadRequest,
		errorCode: NoSuccessfulEvalJob,
		message:   "no successful evaluation task for an experiment",
	}

	ErrorEvalComparisonJobAlreadyArchived = &AutoLearnError{
		httpCode:  http.StatusBadRequest,
		errorCode: EvalComparisonJobAlreadyArchived,
		message:   "eval comparison job already archived",
	}

	ErrorAutoLearnAPIService = &AutoLearnError{
		httpCode:  http.StatusInternalServerError,
		errorCode: AutoLearnAPIUnavailable,
		message:   "autolearn api service call exception",
	}

	ErrorAutoLearnRebuildAlgorithmUnavailable = &AutoLearnError{
		httpCode:  http.StatusBadRequest,
		errorCode: AutoLearnRebuildAlgorithmUnavailable,
		message:   "codebase not found or unavailable",
	}
)
