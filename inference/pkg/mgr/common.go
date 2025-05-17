package mgr

import (
	"encoding/json"
	"errors"
	"fmt"

	"net/url"
	"regexp"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/mongo"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/inference/pkg/types"
	inferenceErr "go.megvii-inc.com/brain/brainpp/projects/aiservice/inference/pkg/utils/ctx/errors"
	modelTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/modelhub/pkg/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/oss"
	aisTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/utils/sds"
)

func GetServiceID(gc *ginlib.GinContext) (string, error) {
	serviceID, err := getRequestParam(gc, InferenceServiceParamID, false)
	if err != nil {
		log.WithError(err).Error("failed to get inference service id from request")
		return "", err
	}

	return serviceID, nil
}

func GetServiceIDAndRvID(gc *ginlib.GinContext) (string, string, error) {
	serviceID, err := GetServiceID(gc)
	if err != nil {
		return "", "", err
	}

	revisionID, err := getRequestParam(gc, InferenceServiceParamRevisionID, false)
	if err != nil {
		log.WithError(err).Error("failed to get inference revision name")
		return "", "", err
	}

	return serviceID, revisionID, nil
}

func GetServiceIDRvIDAndRecordID(gc *ginlib.GinContext) (string, string, string, error) {
	serviceID, rvID, err := GetServiceIDAndRvID(gc)
	if err != nil {
		return "", "", "", err
	}

	recordID, err := getRequestParam(gc, InferenceServiceParamRecordID, false)
	if err != nil {
		log.WithError(err).Error("failed to get conversation recordID")
		return "", "", "", err
	}

	return serviceID, rvID, recordID, nil
}

func (m *Mgr) checkServiceRequest(serviceReq *types.CreateInferenceServiceReq) error {
	if serviceReq.Name == "" {
		log.Error("empty service name")
		return inferenceErr.ErrInvalidParam
	}
	if len(serviceReq.Name) > MaxInferenceNameLength {
		log.Errorf("service name is too long, max length is %d", MaxInferenceNameLength)
		return inferenceErr.ErrInvalidParam
	}
	if !checkInferenceServiceAppType(serviceReq.Approach, serviceReq.AppType) {
		log.Errorf("not supported approach and app type: %s-%s", serviceReq.Approach, serviceReq.AppType)
		return inferenceErr.ErrInvalidParam
	}

	if err := m.checkServiceRvRequest(serviceReq.Revision, serviceReq.AppType); err != nil {
		return err
	}

	return nil
}

func checkInferenceServiceAppType(approach aisTypes.Approach, appType aisTypes.AppType) bool {
	switch approach {
	// 兼容旧版本，20240718之前该值为空
	case aisTypes.ApproachCV, "":
		return appType == aisTypes.AppTypeKPS || appType == aisTypes.AppTypeRegression ||
			appType == aisTypes.AppTypeClassification || appType == aisTypes.AppTypeDetection ||
			appType == aisTypes.AppTypeClassificationAndRegression || appType == aisTypes.AppTypeSegmentation ||
			appType == aisTypes.AppTypeOCR
	case aisTypes.ApproachAIGC:
		return appType == aisTypes.AppTypeImage2Text || appType == aisTypes.AppTypeText2Text
	default:
		return false
	}
}

func (m *Mgr) checkServiceRvRequest(serviceRvReq *types.CreateInferenceServiceRvReq, inferServiceType aisTypes.AppType) error {
	if serviceRvReq == nil {
		log.Error("service revision request is nil")
		return inferenceErr.ErrInvalidParam
	}

	if len(serviceRvReq.ServiceInstances) != 1 {
		log.Error("service instance is not one")
		return inferenceErr.ErrInvalidParam
	}

	if err := checkServiceInstanceReq(serviceRvReq.ServiceInstances[0], inferServiceType, m.ossClient); err != nil {
		return err
	}

	return nil
}

func checkServiceInstanceReq(instance *types.CreateServiceInstanceReq, inferServiceType aisTypes.AppType, ossClient *oss.Client) error {
	if instance == nil {
		log.Error("instance is nil")
		return inferenceErr.ErrInvalidParam
	}

	switch instance.From {
	case types.InferenceInstanceFromModel:
		if err := checkModelParamIsValid(instance, inferServiceType, ossClient); err != nil {
			log.WithError(err).Error("model param is invalid")
			return inferenceErr.ErrInvalidParam
		}
	case types.InferenceInstanceFromImage:
		log.Error("not support image now")
		return inferenceErr.ErrInvalidParam
	default:
		log.Errorf("not support from: %s", instance.From)
		return inferenceErr.ErrInvalidParam
	}

	if instance.AutoShutdown {
		if !insideInt32Range(instance.KeepAliveDuration, types.MinKeepAliveDuration, types.MaxKeepAliveDuration) {
			log.Errorf("invalid keepAliveDuration: %s", string(rune(instance.KeepAliveDuration)))
			return inferenceErr.ErrInvalidParam
		}
	}

	entryPoints := make([]string, 0)
	for i := range instance.EntryPoint {
		if strings.TrimSpace(instance.EntryPoint[i]) != "" {
			entryPoints = append(entryPoints, instance.EntryPoint[i])
		}
	}
	instance.EntryPoint = entryPoints

	envRegex := regexp.MustCompile(types.PodEnvRegex)
	for i := range instance.Env {
		if !envRegex.MatchString(instance.Env[i].Name) {
			log.Errorf("invalid env: %s, must be %s", instance.Env[i].Name, types.PodEnvRegex)
			return inferenceErr.ErrInvalidEnv
		}
	}

	if instance.Resource == nil {
		log.Error("resource not be null")
		return inferenceErr.ErrInvalidParam
	}

	return nil
}

func checkModelParamIsValid(instance *types.CreateServiceInstanceReq, inferServiceType aisTypes.AppType, ossClient *oss.Client) error {
	if inferServiceType != instance.ModelAppType {
		return errors.New("model type is not consistent")
	}

	log.Infof("check model uri: %s", instance.ModelURI)
	if instance.ModelURI == "" {
		return errors.New("model uri is empty")
	}
	u, err := url.Parse(instance.ModelURI)
	if err != nil || u.Host == "" {
		return errors.New("invalid model uri: " + instance.ModelURI)
	}
	if u.Scheme != types.S3Schema {
		return errors.New("only support model from s3, not support: " + instance.ModelURI)
	}
	if err := ossClient.HeadObject(u.Host, u.Path); err != nil {
		return errors.New("model uri can't access or not exist: " + instance.ModelURI)
	}

	if instance.ModelFormatType != "" {
		if instance.ModelFormatType != modelTypes.ModelFormatTypeTM &&
			instance.ModelFormatType != modelTypes.ModelFormatTypeCM &&
			instance.ModelFormatType != modelTypes.ModelFormatTypeONNX &&
			!strings.Contains(string(instance.ModelFormatType), "TM") &&
			instance.ModelFormatType != modelTypes.MOdelFormatTypeSafetensors {
			return fmt.Errorf("invalied model format type: %s", instance.ModelFormatType)
		}
	}
	// todo 从modelHub校验model
	return nil
}

func (m *Mgr) checkInferenceModel(gc *ginlib.GinContext, revision *types.CreateInferenceServiceRvReq) error {
	mr, err := m.modelClient.GetRevision(gc, revision.ServiceInstances[0].ModelID, revision.ServiceInstances[0].ModelRevision)
	if err != nil {
		log.Errorf("failed to get model revision: %s-%s", revision.ServiceInstances[0].ModelID, revision.ServiceInstances[0].ModelRevision)
		return inferenceErr.ErrModelHub
	}

	// todo 其他的不用关心？
	revision.ServiceInstances[0].ModelParentRevision = mr.ParentRevision
	if mr.ParentRevision == "" {
		revision.ServiceInstances[0].Device = "RAW"
		revision.ServiceInstances[0].PlatformDevice = "RAW"
	}

	return nil
}

func getDefaultInferenceService(gc *ginlib.GinContext, serviceReq *types.CreateInferenceServiceReq, config *Config) *types.InferenceService {
	inferService := types.InferenceService{
		ID:          generateServiceID(),
		ProjectID:   gc.GetAuthProjectID(),
		Name:        serviceReq.Name,
		AppType:     serviceReq.AppType,
		Approach:    serviceReq.Approach,
		Description: serviceReq.Description,
		Tags:        serviceReq.Tags,
		CreatedBy:   gc.GetUserName(),
		CreatedAt:   time.Now().Unix(),
		UpdatedBy:   gc.GetUserName(),
		UpdatedAt:   time.Now().Unix(),
	}

	inferService.Revisions = []*types.InferenceServiceRevision{getDefaultServiceRevision(gc, serviceReq.Revision, config)}

	return &inferService
}

func getDefaultServiceRevision(gc *ginlib.GinContext, req *types.CreateInferenceServiceRvReq, config *Config) *types.InferenceServiceRevision {
	return &types.InferenceServiceRevision{
		Revision:         "v1",
		Ready:            false,
		ServiceInstances: []*types.InferenceInstance{getServiceInstance(req.ServiceInstances[0], config)},
		CreatedAt:        time.Now().Unix(),
		CreatedBy:        gc.GetUserName(),
		Stage:            types.InferenceServiceRevisionStageDeploying,
		Hyperparameter:   getDefaultHyparamter(),
	}
}

func getServiceInstance(instanceReq *types.CreateServiceInstanceReq, config *Config) *types.InferenceInstance {
	result := types.InferenceInstance{
		ID: generateServiceID(),
		Meta: &types.InferenceInstanceMeta{
			From:                instanceReq.From,
			ImageURI:            instanceReq.ImageURI,
			ModelID:             instanceReq.ModelID,
			ModelRevision:       instanceReq.ModelRevision,
			ModelParentRevision: instanceReq.ModelParentRevision,
			Platform:            instanceReq.Platform,
			Device:              instanceReq.Device,
			ModelAppType:        string(instanceReq.ModelAppType),
			PlatformDevice:      instanceReq.PlatformDevice,
			ModelFormatType:     instanceReq.ModelFormatType,
			ModelURI:            instanceReq.ModelURI,
			EntryPoint:          instanceReq.EntryPoint,
			Env:                 instanceReq.Env,
		},
		Spec: &types.InferenceInstanceSpec{
			MinReplicas:       1,
			MaxReplicas:       1,
			TrafficPercent:    100,
			AutoShutdown:      instanceReq.AutoShutdown,
			KeepAliveDuration: instanceReq.KeepAliveDuration,
			Resource: types.Resource{
				GPU:     instanceReq.Resource.GPU,
				CPU:     instanceReq.Resource.CPU,
				Memory:  instanceReq.Resource.Memory,
				GPUType: instanceReq.Resource.GPUType,
			},
		},
		Status: &types.InferenceInstanceStatus{
			FailedReason: types.FailedReasonScheduleFailed,
			Ready:        false,
			UpdatedAt:    time.Now().Unix(),
		},
	}

	if instanceReq.Resource.GPUType != "" {
		result.Meta.GPUTypes = []string{instanceReq.Resource.GPUType}
	}
	return &result
}

func getDefaultResources(m *Mgr) *types.DefaultResource {
	return &types.DefaultResource{
		CVModelResource: []*types.Resource{
			{
				GPU:    0,
				CPU:    m.config.DefaultReplicaResource.CPU,
				Memory: m.config.DefaultReplicaResource.Memory,
			},
			&m.config.DefaultReplicaResource,
		},
		ConvertedCVModelResourceMap: map[string][]*types.Resource{
			"CPU": {
				{
					CPU:    m.config.ConvertDeviceConfig.Resource.CPU,
					Memory: m.config.ConvertDeviceConfig.Resource.Memory,
				},
			},
			"2080TI":       {&m.config.ConvertDeviceConfig.Resource},
			"P4":           {&m.config.ConvertDeviceConfig.Resource},
			"A2":           {&m.config.ConvertDeviceConfig.Resource},
			"A100":         {&m.config.ConvertDeviceConfig.Resource},
			"ATLAS300VPRO": {&m.config.ConvertDeviceConfig.Resource},
			"DEFAULT":      {&m.config.DefaultReplicaResource},
		},
		AIGCModelResourceMap: map[string][]*types.Resource{
			"llama3_llama3_8b_full":        {m.config.DefaultAIGCResource["llama3_llama3_8b_full"]},
			"llama3_llama3_8b_chat_full":   {m.config.DefaultAIGCResource["llama3_llama3_8b_full"]},
			"llama3_llama3_8b_freeze":      {m.config.DefaultAIGCResource["llama3_llama3_8b_freeze"]},
			"llama3_llama3_8b_chat_freeze": {m.config.DefaultAIGCResource["llama3_llama3_8b_freeze"]},
			"llama3_llama3_8b_lora":        {m.config.DefaultAIGCResource["llama3_llama3_8b_lora"]},
			"llama3_llama3_8b_chat_lora":   {m.config.DefaultAIGCResource["llama3_llama3_8b_lora"]},
			"qwen2_qwen2_1_8b_lora":        {m.config.DefaultAIGCResource["qwen2_qwen2_1_8b_lora"]},
			"qwen2_qwen2_1_8b_chat_lora":   {m.config.DefaultAIGCResource["qwen2_qwen2_1_8b_lora"]},
			"qwen2_qwen2_1_8b_freeze":      {m.config.DefaultAIGCResource["qwen2_qwen2_1_8b_freeze"]},
			"qwen2_qwen2_1_8b_chat_freeze": {m.config.DefaultAIGCResource["qwen2_qwen2_1_8b_freeze"]},
			"qwen2_qwen2_1_8b_full":        {m.config.DefaultAIGCResource["qwen2_qwen2_1_8b_full"]},
			"qwen2_qwen2_1_8b_chat_full":   {m.config.DefaultAIGCResource["qwen2_qwen2_1_8b_full"]},
			"DEFAULT_lora":                 {m.config.DefaultAIGCResource["DEFAULT_lora"]},
			"DEFAULT_freeze":               {m.config.DefaultAIGCResource["DEFAULT_freeze"]},
			"DEFAULT_full":                 {m.config.DefaultAIGCResource["DEFAULT_full"]},
		},
	}
}

func getDefaultHyparamter() *types.Hyperparameter {
	return &types.Hyperparameter{
		SystemPersonality: "",
		Temperature:       0.1,
		TopP:              0.3,
		PenaltyScore:      1.0,
	}
}

func fillServiceRvRequest(inferService *types.InferenceService, createRv *types.CreateInferenceServiceRvReq) error {
	if len(inferService.Revisions) == 0 || inferService.Revisions[0] == nil {
		return errors.New("inference service is nil")
	}
	if len(inferService.Revisions[0].ServiceInstances) == 0 || inferService.Revisions[0].ServiceInstances[0] == nil {
		return errors.New("inference instance is nil")
	}

	createRv.ServiceInstances[0].KeepAliveDuration = inferService.Revisions[0].ServiceInstances[0].Spec.KeepAliveDuration
	createRv.ServiceInstances[0].AutoShutdown = inferService.Revisions[0].ServiceInstances[0].Spec.AutoShutdown

	return nil
}

func (m *Mgr) fillServiceCommonInfo(gc *ginlib.GinContext, service *types.InferenceService) {
	if service.Approach == "" {
		service.Approach = aisTypes.ApproachCV
	}

	for _, rv := range service.Revisions {
		for _, si := range rv.ServiceInstances {
			modelRevision, err := m.modelClient.GetRevision(gc, si.Meta.ModelID, si.Meta.ModelRevision)
			if err != nil {
				log.Error("failed to get model revision")
				continue
			}

			si.Meta.ModelName = modelRevision.ModelName
			si.Meta.ModelRevisionName = modelRevision.RevisionName
			si.Meta.ModelParentRevisionName = modelRevision.ParentRevisionName
			si.Meta.ModelConvertName = modelRevision.Meta.ConvertName
		}
	}
}

func (m *Mgr) fillServicePodsForLastRevision(service *types.InferenceService) {
	if len(service.Revisions) == 0 || service.Revisions[len(service.Revisions)-1] == nil {
		log.Error("revisions is nil in fillServicePodsForLastRevision")
		return
	}

	lastRevision := service.Revisions[len(service.Revisions)-1]
	if len(lastRevision.ServiceInstances) == 0 || lastRevision.ServiceInstances[0] == nil {
		log.Error("serviceInstance is nil in fillServicePodsForLastRevision")
		return
	}

	lastRevision.ServiceInstances[0].Status.Pods = m.getKFServicePods(service.Name, service.ProjectID)
}

func (m *Mgr) validateServiceStatus(service *types.InferenceService, revisionDesc bool) {
	if len(service.Revisions) == 0 || service.Revisions[len(service.Revisions)-1] == nil {
		log.Error("revisions is nil in validateServiceStatus")
		return
	}

	// 1. reset the status for history revisions
	historyRevisions := service.Revisions[:len(service.Revisions)-1]
	if revisionDesc {
		historyRevisions = service.Revisions[1:]
	}
	for _, revision := range historyRevisions {
		if revision.Active {
			log.Errorf("history revision is active: %s, %s", service.Name, revision.Revision)
			revision.Active = false
		}
	}

	// 2. validate the last revision
	lastRevision := service.Revisions[len(service.Revisions)-1]
	if revisionDesc {
		lastRevision = service.Revisions[0]
	}
	if len(service.Revisions) == int(service.RevisionCount) && !lastRevision.Active {
		log.Errorf("last revision is not active: %s, %s", service.Name, lastRevision.Revision)
		lastRevision.Active = true
	}
	lastRevisionInstance := &types.InferenceInstance{Status: &types.InferenceInstanceStatus{}}
	if len(lastRevision.ServiceInstances) != 0 && lastRevision.ServiceInstances[len(lastRevision.ServiceInstances)-1] != nil {
		lastRevisionInstance = lastRevision.ServiceInstances[len(lastRevision.ServiceInstances)-1]
	}

	// 3. convert state and not ready reason
	lastRevision.State = types.InferenceServiceStateUnknown
	if lastRevision.Stage == types.InferenceServiceRevisionStageDeploying {
		lastRevision.State = types.InferenceServiceStateDeploying
	} else if lastRevision.Stage == types.InferenceServiceRevisionStageDeployFailed {
		lastRevision.State = types.InferenceServiceStateDeployFailed
		lastRevision.NotReadyReason = lastRevisionInstance.Status.NotReadyReason
		lastRevision.FailedReason = types.FailedReasonMap[lastRevisionInstance.Status.FailedReason]
	} else if lastRevision.Stage == types.InferenceServiceRevisionStageShutdown {
		lastRevision.State = types.InferenceServiceStateShutdown
	} else if lastRevision.Stage == types.InferenceServiceRevisionStageServing && lastRevision.Ready {
		lastRevision.State = types.InferenceServiceStateNormal
	} else if lastRevision.Stage == types.InferenceServiceRevisionStageServing && !lastRevision.Ready {
		lastRevision.State = types.InferenceServiceStateAbnormal
		lastRevision.NotReadyReason = lastRevisionInstance.Status.NotReadyReason
		lastRevision.FailedReason = types.FailedReasonMap[lastRevisionInstance.Status.FailedReason]
	}
}

func FillHistoryConversationRecord(col *mongo.Collection, service *types.InferenceService, recordID string, currentReq *types.OpenAIChatReq) ([]byte, error) {
	historyRecord, err := FindConversationRecord(col, recordID)
	if err != nil {
		log.Error("failed to FindConversationRecordByIsvcRvID")
		return nil, err
	}

	mergedConversation := types.OpenAIChatReq{
		Model:            currentReq.Model,
		Messages:         []*types.ChatMessage{},
		FrequencyPenalty: service.Revisions[0].Hyperparameter.PenaltyScore,
		Temperature:      service.Revisions[0].Hyperparameter.Temperature,
		TopP:             service.Revisions[0].Hyperparameter.TopP,
		Stream:           currentReq.Stream,
	}
	if strings.TrimSpace(service.Revisions[0].Hyperparameter.SystemPersonality) != "" {
		mergedConversation.Messages = append(mergedConversation.Messages, &types.ChatMessage{
			Role:    sds.SystemRole,
			Content: service.Revisions[0].Hyperparameter.SystemPersonality,
		})
	}

	for _, c := range historyRecord.Conversations {
		switch c.Modality {
		case sds.TextModality:
			mergedConversation.Messages = append(mergedConversation.Messages, &types.ChatMessage{
				Role:    c.Role,
				Content: c.Content,
			})
		case sds.ImageModality:
			mergedConversation.Messages = append(mergedConversation.Messages, &types.ChatMessage{
				Role: c.Role,
				Content: []*types.ChatMessageContentForTextAndImage{{
					Type: "image_url",
					ImageURL: struct {
						URL string `json:"url"`
					}{
						URL: c.Content,
					},
				}}})
		}
	}

	mergedConversation.Messages = append(mergedConversation.Messages, currentReq.Messages...)
	bytesArr, err := json.Marshal(mergedConversation)
	if err != nil {
		log.Error("failed to marshal conversation")
		return nil, err
	}

	return bytesArr, err
}
