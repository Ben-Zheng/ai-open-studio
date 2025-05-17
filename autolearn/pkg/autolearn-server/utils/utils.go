package utils

import (
	log "github.com/sirupsen/logrus"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/types"
	aisTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
)

// ConvertAutoLearnState 自动学习状态转换 k8s state >>> autoLearn state
func ConvertAutoLearnState(state types.ActionState) (types.AutoLearnState, bool) {
	switch state {
	case types.ActionStateLearning:
		return types.AutoLearnStateLearning, true
	case types.ActionStateCompleted:
		return types.AutoLearnStateCompleted, true
	case types.ActionStateStopped:
		return types.AutoLearnStateStopped, true
	case types.ActionStateFailed:
		return types.AutoLearnStateException, true
	}
	log.WithField("autoLearnState", state).Warn("invalid")
	return -1, false
}

// GetModelFilePathByIterationNum 通过IterationNum获取版本中的modelPath
func GetModelFilePathByIterationNum(iterationNum int32, evaluationDetails []*types.EvaluationDetail) string {
	// 由于重试可能导致同一迭代次数出现多个, 但是部分重试没有传递modelFilePath参数导致重试的evalJobs的modelFilePath为空，所以需要遍历所有。 该问题已修复
	evalJobs := make([]*types.EvaluationDetail, 0)
	for i := range evaluationDetails {
		if evaluationDetails[i].IterationNumber == iterationNum {
			evalJobs = append(evalJobs, evaluationDetails[i])
		}
	}
	if len(evalJobs) == 0 {
		log.Error("invalid iterationNumber")
		return ""
	}

	// 有可能该迭代次数的modelPath未上传或者上传失败
	var modelFilePath string
	for i := range evalJobs {
		if evalJobs[i].ModelFilePath != "" {
			modelFilePath = evalJobs[i].ModelFilePath
			break
		}
	}
	if modelFilePath == "" {
		log.Warn("model file is empty")
	}

	return modelFilePath
}

func AppTypeAndApproachIsValid(req *types.SameCreateRequest) bool {
	switch req.Approach {
	case aisTypes.ApproachCV, "":
		return req.Type == aisTypes.AppTypeClassification || req.Type == aisTypes.AppTypeDetection ||
			req.Type == aisTypes.AppTypeKPS || req.Type == aisTypes.AppTypeRegression ||
			req.Type == aisTypes.AppTypeClassificationAndRegression || req.Type == aisTypes.AppTypeSegmentation ||
			req.Type == aisTypes.AppTypeOCR
	case aisTypes.ApproachAIGC:
		return req.Type == aisTypes.AppTypeText2Text || req.Type == aisTypes.AppTypeImage2Text
	default:
		log.Errorf("invalid approach: %s", req.Approach)
		return false
	}
}

func AutolearnStateInFinal(state types.AutoLearnState) bool {
	for _, fstate := range types.FinalState {
		if state == fstate {
			return true
		}
	}
	return false
}
