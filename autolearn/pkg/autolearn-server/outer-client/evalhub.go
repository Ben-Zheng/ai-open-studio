package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson/primitive"

	authTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/auth/pkg/app/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/config"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/ctx/errors"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/utils"
	evalTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/evalhub/pkg/types"
	evalErrors "go.megvii-inc.com/brain/brainpp/projects/aiservice/evalhub/pkg/types/ctx/errors"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/authlib"
	aisTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
)

const (
	evalHubURLPrefix = "api/v1"

	// 请求evalHub接口超时时间，单位s
	evalHubRequestTimeout = 120

	EvalJobResultStateFailed = evalTypes.EvalJobResultStateFailed

	// 评测任务Job的资源配置
	EvalJobCPU    = 8
	EvalJobGPU    = 0
	EvalJobMemory = 64
)

type EvalCreateReqBody struct {
	Name               string                      `json:"name"`
	Desc               string                      `json:"desc"`
	Tags               []string                    `json:"tags"`
	EvalType           string                      `json:"evalType" enums:"Default, SDS"`
	Approach           aisTypes.Approach           `json:"approach"`
	AppType            aisTypes.AppType            `json:"appType"`
	Codebase           *evalTypes.BriefCodebase    `json:"codebase"`
	SDSFiles           []*evalTypes.SDSMeta        `json:"sdsFiles"`
	Resources          *types.Resource             `json:"resources"`
	EstimateDuration   int64                       `json:"estimateDuration"`
	EvalSource         evalTypes.EvalSource        `json:"evalSource"`
	QuotaConfig        *evalTypes.QuotaConfig      `json:"quotaConfig"`
	Parameters         []*evalTypes.ParameterSpec  `json:"parameters"`
	IOUOptions         []float64                   `json:"iouOptions"`                                   // 评测 IOU 选项
	DetectedObjectType aisTypes.DetectedObjectType `json:"detectedObjectType" enums:"Normal,MultiMatch"` // 当应用类型为检测时，选择目标类型，普通物体或边界不规则物体
}

type EvalJobCreateReqBody struct {
	SDSFiles           []*evalTypes.SDSMeta        `json:"sdsFiles"`
	DetectedObjectType aisTypes.DetectedObjectType `json:"detectedObjectType" enums:"Normal,MultiMatch"` // 当应用类型为检测时，选择目标类型，普通物体或边界不规则物体
}

type EvalComparisonReqBody struct {
	AppType aisTypes.AppType `json:"appType"`
	JobIDs  []string         `json:"jobIDs"`
}

func buildSDSMeta(revision *types.AutoLearnRevision, sdsS3URI string) *evalTypes.SDSMeta {
	sdsMeta := &evalTypes.SDSMeta{
		S3URI:       sdsS3URI,
		DatasetName: fmt.Sprintf("sdsevaluation%s", utils.RandStringRunesWithLower(6)),
		ModelName:   fmt.Sprintf("sdsevaluation%s", utils.RandStringRunesWithLower(6)),
	}
	// 按照目前的情况自动学习数据集只存在三种情况
	// 1. SDS(Train) + SDS(Validation)
	// 2. Pair(Train) + Pair(Validation)
	// 3. Dataset(Train) + Dataset(Validation)
	var validationDataset *types.Dataset
	for _, d := range revision.Datasets {
		if d.Type == aisTypes.DatasetTypeValid {
			validationDataset = d
			break
		}
	}
	if validationDataset == nil {
		return sdsMeta
	}

	if validationDataset.SourceType == types.SourceTypeSDS {
		sdsMeta.Dataset = &evalTypes.BriefDataset{
			SourceType:  evalTypes.DatasetSourceTypeSDS,
			DatasetName: validationDataset.SDSPath,
			SDSURL:      validationDataset.SDSPath,
		}
	} else if validationDataset.SourceType == types.SourceTypeDataset {
		sdsMeta.Dataset = &evalTypes.BriefDataset{
			SourceType:   evalTypes.DatasetSourceTypeDataset,
			DatasetID:    validationDataset.DatasetMeta.ID,
			RevisionID:   validationDataset.DatasetMeta.RevisionID,
			OriginLevel:  validationDataset.DatasetMeta.OriginLevel,
			DatasetName:  validationDataset.DatasetMeta.Name,
			RevisionName: validationDataset.DatasetMeta.RevisionName,
			SDSURL:       validationDataset.SDSPath,
		}
	} else if validationDataset.SourceType == types.SourceTypePair {
		sdsMeta.Dataset = &evalTypes.BriefDataset{
			SourceType:   evalTypes.DatasetSourceTypePair,
			DatasetID:    validationDataset.DatasetMeta.PairID,
			RevisionID:   validationDataset.DatasetMeta.PairRevisionID,
			OriginLevel:  validationDataset.DatasetMeta.OriginLevel,
			DatasetName:  validationDataset.DatasetMeta.PairName,
			RevisionName: validationDataset.DatasetMeta.PairRevisionName,
			SDSURL:       validationDataset.SDSPath,
		}
	}
	return sdsMeta
}

// CreateEvaluation 创建评测任务
func CreateEvaluation(autoLearn *types.AutoLearn, revision *types.AutoLearnRevision, sdsS3URI string) (*types.EvaluationDetail, error) {
	ctx, cancel := context.WithTimeout(context.Background(), evalHubRequestTimeout*time.Second)
	defer cancel()

	logs := log.WithField("autoLearn", fmt.Sprintf("%s/%s", autoLearn.AutoLearnID, revision.RevisionID))

	// custom image can find from revision.Algorithms by id

	createBody := EvalCreateReqBody{
		Name:               utils.GenerateNameForOtherService(autoLearn.AutoLearnName, revision.RevisionName, utils.RandStringRunesWithLower(6)),
		Tags:               []string{"autoLearn", string(revision.Type)},
		Desc:               "I come from far away",
		EvalType:           evalTypes.EvalTypeSDS,
		Approach:           revision.Approach,
		AppType:            revision.Type,
		SDSFiles:           []*evalTypes.SDSMeta{buildSDSMeta(revision, sdsS3URI)},
		EstimateDuration:   1,
		Parameters:         make([]*evalTypes.ParameterSpec, 0),
		EvalSource:         evalTypes.EvalSourceAutoLearnAuto,
		DetectedObjectType: revision.DetectedObjectType,
	}

	if revision.QuotaGroup != "" {
		temp := strings.Split(revision.QuotaGroup, ":")
		createBody.QuotaConfig = &evalTypes.QuotaConfig{QuotaGroup: temp[len(temp)-1], QuotaBestEffort: "no", QuotaPreemptible: "no"}
	}

	if revision.EvalMetric != nil {
		createBody.IOUOptions = []float64{revision.EvalMetric.IOU}
	}

	bodyBytes, _ := json.Marshal(createBody)
	req, err := http.NewRequest("POST",
		fmt.Sprintf("%s/%s/%s", config.Profile.AisEndPoint.EvalHub, evalHubURLPrefix, "evaluations"), bytes.NewReader(bodyBytes))
	if err != nil {
		logs.WithField("EvalCreateReqBody", createBody).WithError(err).Error("failed to create evaluation request")
		return nil, err
	}

	token := authlib.GenerateBasicAuthToken(config.Profile.AisAccessToken.AccessKey, config.Profile.AisAccessToken.SecretKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(authlib.KBHeaderAuthorization, token)
	req.Header.Set(authTypes.AISProjectHeader, autoLearn.ProjectID)
	req.Header.Set(authTypes.AISTenantHeader, autoLearn.TenantID)
	req.Header.Set(authlib.HeaderUserID, revision.CreatedBy.UserID)
	req.Header.Set(authlib.HeaderUserName, revision.CreatedBy.UserName)

	resp, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		logs.WithError(err).Error("failed to create evaluation")
		return nil, errors.ErrorEvalHubService
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logs.Errorf("response.status.code: %d", resp.StatusCode)
		return nil, errors.ErrorEvalHubService
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logs.Error("failed to read response body")
		return nil, err
	}

	respData := struct {
		Data *evalTypes.Evaluation `json:"data"`
	}{
		&evalTypes.Evaluation{},
	}
	if err = json.Unmarshal(body, &respData); err != nil {
		logs.WithError(err).Error("failed to unmarshal response body")
		return nil, err
	}

	if len(respData.Data.Jobs) == 0 {
		logs.WithField("respData", respData).Error("evaluation jobs is zero")
		return nil, errors.ErrorEvalHubService
	}

	var jobs []*types.EvaluationJobDetail
	for _, job := range respData.Data.Jobs {
		jobs = append(jobs, &types.EvaluationJobDetail{
			EvaluationJAobState: job.State,
			EvaluationRJobName:  job.RJobName,
			AppType:             job.AppType,
			EvaluationJobID:     job.ID.Hex(),
		})
	}

	return &types.EvaluationDetail{
		EvaluationID:       respData.Data.ID.Hex(),
		EvaluationName:     respData.Data.Name,
		EvaluationJobID:    respData.Data.Jobs[0].ID.Hex(),
		EvaluationRJobName: respData.Data.Jobs[0].RJobName,
		EvaluationJobState: respData.Data.Jobs[0].State,
		EvaluationJobs:     jobs,
		SdsFilePath:        sdsS3URI,
		IsDeleted:          false,
	}, nil
}

// CreateEvaluationJob 创建评测子任务
func CreateEvaluationJob(autoLearn *types.AutoLearn, revision *types.AutoLearnRevision, evaluationID, evaluationName, sdsS3URI string) (*types.EvaluationDetail, error) {
	ctx, cancel := context.WithTimeout(context.Background(), evalHubRequestTimeout*time.Second)
	defer cancel()

	logs := log.WithField("autolearn", fmt.Sprintf("%s/%s", autoLearn.AutoLearnID, revision.RevisionID))

	createBody := EvalJobCreateReqBody{
		SDSFiles:           []*evalTypes.SDSMeta{buildSDSMeta(revision, sdsS3URI)},
		DetectedObjectType: revision.DetectedObjectType,
	}
	bodyBytes, _ := json.Marshal(createBody)

	req, err := http.NewRequest("POST",
		fmt.Sprintf("%s/%s/evaluations/%s", config.Profile.AisEndPoint.EvalHub, evalHubURLPrefix, evaluationID), bytes.NewReader(bodyBytes))
	if err != nil {
		logs.WithField("EvalCreateReqBody", createBody).WithError(err).Error("failed to create evaluation request")
		return nil, err
	}

	token := authlib.GenerateBasicAuthToken(config.Profile.AisAccessToken.AccessKey, config.Profile.AisAccessToken.SecretKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(authlib.KBHeaderAuthorization, token)
	req.Header.Set(authTypes.AISProjectHeader, autoLearn.ProjectID)
	req.Header.Set(authTypes.AISTenantHeader, autoLearn.TenantID)
	req.Header.Set(authlib.HeaderUserID, revision.CreatedBy.UserID)
	req.Header.Set(authlib.HeaderUserName, revision.CreatedBy.UserName)

	resp, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		logs.WithError(err).Error("failed to create evaluation")
		return nil, errors.ErrorEvalHubService
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logs.Errorf("response.status.code: %d", resp.StatusCode)
		return nil, errors.ErrorEvalHubService
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logs.Error("failed to read response body")
		return nil, err
	}

	respData := struct {
		Data []*evalTypes.Job `json:"data"`
	}{
		[]*evalTypes.Job{},
	}
	if err = json.Unmarshal(body, &respData); err != nil {
		logs.WithError(err).Error("failed to unmarshal response body")
		return nil, err
	}

	if len(respData.Data) == 0 {
		logs.WithField("respData", respData).Error("evaluation jobs is zero")
		return nil, errors.ErrorEvalHubService
	}

	var jobs []*types.EvaluationJobDetail
	for _, job := range respData.Data {
		jobs = append(jobs, &types.EvaluationJobDetail{
			EvaluationJAobState: job.State,
			EvaluationRJobName:  job.RJobName,
			AppType:             job.AppType,
			EvaluationJobID:     job.ID.Hex(),
		})
	}

	return &types.EvaluationDetail{
		EvaluationID:       evaluationID,
		EvaluationName:     evaluationName,
		EvaluationJobID:    respData.Data[0].ID.Hex(),
		EvaluationRJobName: respData.Data[0].RJobName,
		EvaluationJobState: respData.Data[0].State,
		EvaluationJobs:     jobs,
		SdsFilePath:        sdsS3URI,
		IsDeleted:          false,
	}, nil
}

// GetEvaluationJob 获取评测子任务
func GetEvaluationJob(tenantID, projectID, evaluationID, evaluationJobID string) (*evalTypes.Job, error) {
	ctx, cancel := context.WithTimeout(context.Background(), evalHubRequestTimeout*time.Second)
	defer cancel()

	req, err := http.NewRequest("GET",
		fmt.Sprintf("%s/%s/evaluations/%s/jobs/%s", config.Profile.AisEndPoint.EvalHub, evalHubURLPrefix, evaluationID, evaluationJobID), nil)
	if err != nil {
		log.WithError(err).Error("failed to create evaluation request")
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(authlib.KBHeaderAuthorization, authlib.GenerateBasicAuthToken(config.Profile.AisAccessToken.AccessKey, config.Profile.AisAccessToken.SecretKey))
	req.Header.Set(authTypes.AISProjectHeader, projectID)
	req.Header.Set(authTypes.AISTenantHeader, tenantID)

	resp, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		log.WithError(err).Errorf("failed to request evaluation: %+v", req)
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.WithField("request", req).Errorf("response.status.code: %d", resp.StatusCode)
		return nil, errors.ErrorEvalHubService
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error("failed to read response body")
		return nil, err
	}

	respData := struct {
		Data *evalTypes.Job `json:"data"`
	}{
		&evalTypes.Job{},
	}
	if err = json.Unmarshal(body, &respData); err != nil {
		log.WithError(err).Error("failed to unmarshal response body")
		return nil, err
	}

	return respData.Data, nil
}

// CreateComparison 创建评测对比
func CreateComparison(tenantID, projectID string, user *types.User, createBody *EvalComparisonReqBody) (string, any, error) {
	ctx, cancel := context.WithTimeout(context.Background(), evalHubRequestTimeout*time.Second)
	defer cancel()

	logs := log.WithField("autoLearn", "")

	bodyBytes, _ := json.Marshal(createBody)
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/%s/%s", config.Profile.AisEndPoint.EvalHub, evalHubURLPrefix, "comparisons"), bytes.NewReader(bodyBytes))
	if err != nil {
		logs.WithField("EvalComparisonCreateReqBody", createBody).WithError(err).Error("failed to create evaluation comparison request")
		return "", nil, err
	}

	token := authlib.GenerateBasicAuthToken(config.Profile.AisAccessToken.AccessKey, config.Profile.AisAccessToken.SecretKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(authlib.KBHeaderAuthorization, token)
	req.Header.Set(authTypes.AISProjectHeader, projectID)
	req.Header.Set(authTypes.AISTenantHeader, tenantID)
	req.Header.Set(authlib.HeaderUserID, user.UserID)
	req.Header.Set(authlib.HeaderUserName, user.UserName)

	resp, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		logs.WithError(err).Error("failed to create evaluation comparison")
		return "", nil, errors.ErrorEvalHubService
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusBadRequest {
		logs.Errorf("bad code, response.status.code: %d", resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			logs.Error("bad code, failed to read response body")
			return "", nil, err
		}

		respData := struct {
			Message string `json:"message"`
			Subcode string `json:"subcode"`
			Data    any    `json:"data"`
		}{}
		if err = json.Unmarshal(body, &respData); err != nil {
			logs.WithError(err).Error("bad code, failed to unmarshal response body")
			return "", nil, err
		}
		logs.Infof("bad code, respData: %+v", respData)

		if respData.Subcode == string(evalErrors.JobsNotUnArchived) {
			return "", respData.Data, errors.ErrorEvalJobNotUnArchived
		}
		return "", nil, errors.ErrorEvalHubService
	}

	if resp.StatusCode != http.StatusOK {
		logs.Errorf("response.status.code: %d", resp.StatusCode)
		return "", nil, errors.ErrorEvalHubService
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logs.Error("failed to read response body")
		return "", nil, err
	}

	respData := struct {
		Data *evalTypes.ComparisonWithSnapshot `json:"data"`
	}{
		&evalTypes.ComparisonWithSnapshot{},
	}
	if err = json.Unmarshal(body, &respData); err != nil {
		logs.WithError(err).Error("failed to unmarshal response body")
		return "", nil, err
	}

	if respData.Data.Comparison == nil {
		logs.WithField("respData", respData).Error("comparison is nil")
		return "", nil, errors.ErrorEvalHubService
	}

	log.Debugf("[CXY] comparison creation response: %+v", respData.Data.Comparison)
	log.Debugf("[CXY] comparison creation response user: %+v", respData.Data.Comparison.CreatedBy)
	return respData.Data.Comparison.ID.Hex(), nil, nil
}

// CreateComparisonSnapshot 创建评测对比快照
func CreateComparisonSnapshot(tenantID, projectID string, user *types.User, createBody *evalTypes.CreateComparisonSnapshotRequest) (comparisonID, comparisonSnapshotID primitive.ObjectID, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), evalHubRequestTimeout*time.Second)
	defer cancel()

	logs := log.WithField("autoLearn", "")

	bodyBytes, _ := json.Marshal(createBody)
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/%s/%s", config.Profile.AisEndPoint.EvalHub, evalHubURLPrefix, "comparison-snapshots"), bytes.NewReader(bodyBytes))
	if err != nil {
		logs.WithField("ComparisonSnapshotCreateReqBody", createBody).WithError(err).Error("failed to create evaluation comparison snapshot request")
		return
	}

	token := authlib.GenerateBasicAuthToken(config.Profile.AisAccessToken.AccessKey, config.Profile.AisAccessToken.SecretKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(authlib.KBHeaderAuthorization, token)
	req.Header.Set(authTypes.AISProjectHeader, projectID)
	req.Header.Set(authTypes.AISTenantHeader, tenantID)
	req.Header.Set(authlib.HeaderUserID, user.UserID)
	req.Header.Set(authlib.HeaderUserName, user.UserName)

	resp, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		logs.WithError(err).Error("failed to create evaluation snapshot")
		err = errors.ErrorEvalHubService
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logs.Errorf("response.status.code: %d", resp.StatusCode)
		err = errors.ErrorEvalHubService
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logs.Error("failed to read response body")
		return
	}

	respData := struct {
		Data *evalTypes.ComparisonWithSnapshot `json:"data"`
	}{
		&evalTypes.ComparisonWithSnapshot{},
	}
	if err = json.Unmarshal(body, &respData); err != nil {
		logs.WithError(err).Error("failed to unmarshal response body")
		return
	}

	if respData.Data.Comparison == nil || respData.Data.ComparisonSnapshot == nil {
		logs.WithField("respData", respData).Error("comparison or snapshot is nil")
		err = errors.ErrorEvalHubService
		return
	}

	log.Debugf("[CXY] comparison creation response: %+v", respData.Data.Comparison)
	log.Debugf("[CXY] comparison creation response user: %+v", respData.Data.Comparison.CreatedBy)
	return respData.Data.Comparison.ID, respData.Data.ComparisonSnapshot.ID, nil
}

// ExportComparison 导出评测对比
func ExportComparison(tenantID, projectID string, user *types.User, comparisonID string, createBody evalTypes.ExportComparisonSnapshotRequest) (result *evalTypes.ComparisonWithSnapshot, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), evalHubRequestTimeout*time.Second)
	defer cancel()

	logs := log.WithField("autoLearn", "")
	param := make(map[string]any)
	paramBytes, _ := json.Marshal(createBody)
	json.Unmarshal(paramBytes, &param)
	req, err := http.NewRequest("GET",
		fmt.Sprintf("%s/%s/%s/%s/exportData", config.Profile.AisEndPoint.EvalHub, evalHubURLPrefix, "comparisons", comparisonID), nil)
	q := req.URL.Query()
	q.Add("iou", strconv.FormatFloat(createBody.IOU, 'f', 1, 64))
	q.Add("attribute", createBody.Attribute)
	q.Add("groupBy", string(createBody.GroupBy))
	q.Add("type", "")
	for _, s := range createBody.Sorts {
		ss, _ := json.Marshal(s)
		q.Add("sorts", string(ss))
	}
	for _, b := range createBody.Bases {
		q.Add("bases", b)
	}
	req.URL.RawQuery = q.Encode()
	if err != nil {
		logs.WithField("ComparisonSnapshotCreateReqBody", createBody).WithError(err).Error("failed to create evaluation comparison snapshot request")
		return
	}

	token := authlib.GenerateBasicAuthToken(config.Profile.AisAccessToken.AccessKey, config.Profile.AisAccessToken.SecretKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(authlib.KBHeaderAuthorization, token)
	req.Header.Set(authTypes.AISProjectHeader, projectID)
	req.Header.Set(authTypes.AISTenantHeader, tenantID)
	req.Header.Set(authlib.HeaderUserID, user.UserID)
	req.Header.Set(authlib.HeaderUserName, user.UserName)

	resp, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		logs.WithError(err).Error("failed to create evaluation snapshot")
		err = errors.ErrorEvalHubService
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logs.Errorf("response.status.code: %d", resp.StatusCode)
		err = errors.ErrorEvalHubService
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logs.Error("failed to read response body")
		return
	}

	respData := struct {
		Data *evalTypes.ComparisonWithSnapshot `json:"data"`
	}{
		&evalTypes.ComparisonWithSnapshot{},
	}
	if err = json.Unmarshal(body, &respData); err != nil {
		logs.WithError(err).Error("failed to unmarshal response body")
		return
	}

	if respData.Data.Comparison == nil {
		logs.WithField("respData", respData).Error("comparison is nil")
		err = errors.ErrorEvalHubService
		return
	}

	log.Debugf("[CXY] comparison creation response comparison: %+v", respData.Data.Comparison)
	log.Debugf("[CXY] comparison creation response dataGroups: %+v", respData.Data.DatasetGroups)
	return respData.Data, nil
}

// GetComparison 获取评测对比（当前主要是校验状态）
func GetComparison(tenantID, projectID string, user *types.User, comparisonID string) (result *evalTypes.ComparisonWithSnapshot, errorData *evalTypes.ComparisonErrorData, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), evalHubRequestTimeout*time.Second)
	defer cancel()

	logs := log.WithField("autoLearn", "")
	req, err := http.NewRequest("GET",
		fmt.Sprintf("%s/%s/%s/%s", config.Profile.AisEndPoint.EvalHub, evalHubURLPrefix, "comparisons", comparisonID), nil)
	if err != nil {
		logs.WithError(err).Error("failed to create evaluation comparison get request")
		return
	}

	token := authlib.GenerateBasicAuthToken(config.Profile.AisAccessToken.AccessKey, config.Profile.AisAccessToken.SecretKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(authlib.KBHeaderAuthorization, token)
	req.Header.Set(authTypes.AISProjectHeader, projectID)
	req.Header.Set(authTypes.AISTenantHeader, tenantID)
	req.Header.Set(authlib.HeaderUserID, user.UserID)
	req.Header.Set(authlib.HeaderUserName, user.UserName)

	resp, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		logs.WithError(err).Error("failed to create evaluation snapshot")
		err = errors.ErrorEvalHubService
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logs.Error("failed to read response body")
		return
	}

	if resp.StatusCode == http.StatusBadRequest {
		errroResp := struct {
			Message string                         `json:"message"`
			SubCode string                         `json:"subCode"`
			Data    *evalTypes.ComparisonErrorData `json:"data"`
		}{
			Data: new(evalTypes.ComparisonErrorData),
		}
		if err = json.Unmarshal(body, &errroResp); err != nil {
			logs.WithError(err).Error("failed to unmarshal response body")
			return
		}
		if errroResp.SubCode == string(evalErrors.JobsNotUnArchived) {
			return nil, errroResp.Data, errors.ErrorEvalHubService
		}
	}

	if resp.StatusCode != http.StatusOK {
		logs.Errorf("response.status.code: %d", resp.StatusCode)
		err = errors.ErrorEvalHubService
		return
	}

	respData := struct {
		Data *evalTypes.ComparisonWithSnapshot `json:"data"`
	}{
		&evalTypes.ComparisonWithSnapshot{},
	}
	if err = json.Unmarshal(body, &respData); err != nil {
		logs.WithError(err).Error("failed to unmarshal response body")
		return
	}

	if respData.Data.Comparison == nil {
		logs.WithField("respData", respData).Error("comparison is nil")
		err = errors.ErrorEvalHubService
		return
	}

	log.Debugf("[CXY] comparison get response comparison: %+v", respData.Data.Comparison)
	log.Debugf("[CXY] comparison get response dataGroups: %+v", respData.Data.DatasetGroups)
	return respData.Data, nil, nil
}

// GetMetricsMetaOptions 获取评测meta-options
func GetMetricsMetaOptions(tenantID, projectID, evaluationID, evaluationJobID string) (*evalTypes.MetricMetaOptions, error) {
	ctx, cancel := context.WithTimeout(context.Background(), evalHubRequestTimeout*time.Second)
	defer cancel()

	req, err := http.NewRequest("GET",
		fmt.Sprintf("%s/%s/evaluations/%s/jobs/%s/metrics/meta-options", config.Profile.AisEndPoint.EvalHub, evalHubURLPrefix, evaluationID, evaluationJobID), nil)
	if err != nil {
		log.WithError(err).Error("failed to create evaluation request")
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(authlib.KBHeaderAuthorization, authlib.GenerateBasicAuthToken(config.Profile.AisAccessToken.AccessKey, config.Profile.AisAccessToken.SecretKey))
	req.Header.Set(authTypes.AISProjectHeader, projectID)
	req.Header.Set(authTypes.AISTenantHeader, tenantID)

	resp, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		log.WithError(err).Errorf("failed to request evaluation: %+v", req)
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.WithField("request", req).Errorf("response.status.code: %d", resp.StatusCode)
		return nil, errors.ErrorEvalHubService
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error("failed to read response body")
		return nil, err
	}

	respData := struct {
		Data *evalTypes.MetricMetaOptions `json:"data"`
	}{
		&evalTypes.MetricMetaOptions{},
	}
	if err = json.Unmarshal(body, &respData); err != nil {
		log.WithError(err).Error("failed to unmarshal response body")
		return nil, err
	}

	return respData.Data, nil
}

type MetricBasicAnalysisChart struct {
	IOU       float64 `json:"iou"`
	Attribute string  `json:"attribute"`
	Key       string  `json:"key"`
}

// GetMetricBasicAnalysisChart 获取折线图
func GetMetricBasicAnalysisChart(tenantID, projectID, evaluationID, evaluationJobID string, query *MetricBasicAnalysisChart) (*evalTypes.Chart, error) {
	ctx, cancel := context.WithTimeout(context.Background(), evalHubRequestTimeout*time.Second)
	defer cancel()

	req, err := http.NewRequest("GET",
		fmt.Sprintf("%s/%s/evaluations/%s/jobs/%s/metrics/basic-analysis/chart", config.Profile.AisEndPoint.EvalHub, evalHubURLPrefix, evaluationID, evaluationJobID), nil)
	if err != nil {
		log.WithError(err).Error("failed to create evaluation request")
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(authlib.KBHeaderAuthorization, authlib.GenerateBasicAuthToken(config.Profile.AisAccessToken.AccessKey, config.Profile.AisAccessToken.SecretKey))
	req.Header.Set(authTypes.AISProjectHeader, projectID)
	req.Header.Set(authTypes.AISTenantHeader, tenantID)

	q := req.URL.Query()
	q.Add("iou", strconv.FormatFloat(query.IOU, 'f', 1, 64))
	q.Add("attribute", query.Attribute)
	q.Add("key", query.Key)
	req.URL.RawQuery = q.Encode()

	resp, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		log.WithError(err).Errorf("failed to request evaluation: %+v", req)
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.WithField("request", req).Errorf("response.status.code: %d", resp.StatusCode)
		return nil, errors.ErrorEvalHubService
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error("failed to read response body")
		return nil, err
	}

	respData := struct {
		Data *evalTypes.Chart `json:"data"`
	}{
		&evalTypes.Chart{},
	}
	if err = json.Unmarshal(body, &respData); err != nil {
		log.WithError(err).Error("failed to unmarshal response body")
		return nil, err
	}

	return respData.Data, nil
}
