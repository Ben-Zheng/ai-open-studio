package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/config"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/ctx/errors"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/types"
	recommendType "go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/recommend-server/types"
	aisConst "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
)

const (
	// 请求recommend-server接口超时时间，单位s
	recommendAlgorithmRequestTimeout = 60 * 8

	// 获取推荐算法
	getRecommendAlgorithmPath = "api/v1/solutions/recommend-with-reasons"

	// 获取SDS文件中类别集合
	getClassSummarizePath = "snapdet/dataset/summarize"
)

type RecommendAlgorithmReq struct {
	TrainType                aisConst.AppType
	SolutionNum              int32
	SpeedMetric              *types.SpeedMetric
	ModelType                string
	SolutionName             string
	SolutionhubRecommendBody *types.RecommendRequestBody
}

// GetRecommendAlgorithm 获取推荐算法
func GetRecommendAlgorithm(reqParams *RecommendAlgorithmReq) ([]*types.Algorithm, error) {
	ctx, cancel := context.WithTimeout(context.Background(), recommendAlgorithmRequestTimeout*time.Second)
	defer cancel()

	reqParams.SolutionhubRecommendBody.SolutionNumber = int(reqParams.SolutionNum)
	if reqParams.TrainType == aisConst.AppTypeDetection ||
		reqParams.TrainType == aisConst.AppTypeKPS ||
		reqParams.TrainType == aisConst.AppTypeClassification ||
		reqParams.TrainType == aisConst.AppTypeSegmentation {
		if reqParams.SpeedMetric != nil && reqParams.SpeedMetric.ProcessTimePerImg != nil {
			reqParams.SolutionhubRecommendBody.LatencyMax = reqParams.SpeedMetric.ProcessTimePerImg
		}
	}

	reqParams.SolutionhubRecommendBody.Type = string(reqParams.TrainType)
	reqParams.SolutionhubRecommendBody.SupportedDevice = reqParams.ModelType
	reqParams.SolutionhubRecommendBody.SolutionName = reqParams.SolutionName
	jsonValue, err := json.Marshal(reqParams.SolutionhubRecommendBody)
	if err != nil {
		fmt.Println("Error marshaling JSON:", err)
		return nil, err
	}

	requestBody := bytes.NewBuffer(jsonValue)
	serverAddr := config.Profile.AisEndPoint.SolutionHub
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/%s", serverAddr, getRecommendAlgorithmPath), requestBody)
	if err != nil {
		log.WithError(err).Error("failed to create new recommendServer request")
		return nil, err
	}

	log.Infof("Get recommend algorithm uri: %s", req.URL.String())
	resp, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		log.WithError(err).Error("failed to request recommend server")
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.WithError(err).Error("failed to read response body")
		return nil, err
	}

	algorithmResp := struct {
		MismatchReasons []*struct {
			Reason string `json:"reason"`
		} `json:"mismatchReasons"`
		Solutions []*types.AlgorithmSolutionhub `json:"solutions"`
	}{}
	if err := json.Unmarshal(body, &algorithmResp); err != nil {
		log.WithError(err).Error("failed to unmarshal body")
		return nil, err
	}

	var reasons []string
	for _, reas := range algorithmResp.MismatchReasons {
		reasons = append(reasons, reas.Reason)
	}

	er := fmt.Errorf("%s", strings.Join(reasons, "/n"))
	algors := []*types.Algorithm{}
	if algorithmResp.Solutions != nil {
		for _, hubSol := range algorithmResp.Solutions {
			alg := makeAlgorithmFromSolutions(hubSol)
			algors = append(algors, alg)
		}
	}
	return algors, er
}

func GetSDSFileClass(trainType aisConst.AppType, valSDSPath string) ([]string, int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), recommendAlgorithmRequestTimeout*time.Second)
	defer cancel()

	serverAddr := config.Profile.AisEndPoint.AutoLearnSampler
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/%s", serverAddr, getClassSummarizePath), nil)
	if err != nil {
		log.WithError(err).Error("failed to creat new recommendServer request")
		return nil, 0, err
	}

	q := req.URL.Query()
	q.Add(recommendType.QUERY_TRAIN_TYPE, string(trainType))
	q.Add(recommendType.QUERY_VAL_SDS, valSDSPath)
	req.URL.RawQuery = q.Encode()

	log.Debugf("uri: %s", req.URL.String())

	resp, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		log.WithError(err).Error("failed to request classes")
		return nil, 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.WithError(err).Error("failed to read response body")
		return nil, 0, err
	}

	if resp.StatusCode != http.StatusOK {
		log.WithField("request", req).Errorf("response.status.code: %d", resp.StatusCode)
		return nil, resp.StatusCode, errors.ErrorGetClassFailed
	}

	classes := struct {
		ClassNames []string `json:"class_names"`
	}{ClassNames: make([]string, 0)}
	if err := json.Unmarshal(body, &classes); err != nil {
		log.WithError(err).Error("failed to unmarshal body")
		return nil, 0, err
	}

	return classes.ClassNames, resp.StatusCode, nil
}

func makeAlgorithmFromSolutions(soltuion *types.AlgorithmSolutionhub) *types.Algorithm {
	hyp := types.HyperParamDefine{}
	removeEsc := strings.ReplaceAll(soltuion.HyperParamDefine, `\"`, `"`)
	json.Unmarshal([]byte(removeEsc), &hyp)
	properties := []*types.Property{}

	IMAGE_SIZE_MAP := map[string]string{
		"Detection":                   "768x1280",
		"Classification":              "128x128",
		"ClassificationAndRegression": "128x128",
		"Regression":                  "128x128",
		"Keypoint":                    "384x384",
		"Segmentation":                "960x960",
	}

	profilingAny := soltuion.Profiling["speed"]
	profiling := []*types.AlgorithmSolutionhubProfiling{}
	if v, ok := profilingAny.([]*types.AlgorithmSolutionhubProfiling); ok {
		profiling = v
	}

	var flopsSpec string
	if len(profiling) > 0 {
		flopsSpec = profiling[0].ImageSpecs
	} else {
		flopsSpec = IMAGE_SIZE_MAP[soltuion.Type]
	}

	properties = append(properties, &types.Property{
		Key:         "flops",
		Value:       1024,
		Description: "该模型的浮点数计算量",
		ImageSpec:   flopsSpec,
	})
	for _, pr := range profiling {
		if pr.InferTime != -1 {
			properties = append(properties, &types.Property{
				Key:         fmt.Sprintf("infer@%s@%s", pr.ImageSpecs, fmt.Sprintf("%f", pr.InferTime)),
				Value:       pr.InferTime,
				GPU:         pr.Device,
				Unit:        "ms",
				Description: fmt.Sprintf("以%s为推理后端推理%s的图片所需时间", pr.Device, pr.ImageSpecs),
				ImageSpec:   pr.ImageSpecs,
			})
		}
		if pr.MaxDeviceMem != -1 {
			properties = append(properties, &types.Property{
				Key:         fmt.Sprintf("memory@%s@%s", pr.ImageSpecs, fmt.Sprintf("%f", pr.InferTime)),
				Value:       pr.InferTime,
				GPU:         pr.Device,
				Unit:        "MB",
				Description: fmt.Sprintf("以%s为推理后端推理%s的图片占用的内存", pr.Device, pr.ImageSpecs),
				ImageSpec:   pr.ImageSpecs,
			})
		}
	}

	params := map[string]any{}
	for k, v := range hyp.Properties {
		params[k] = v["default"]
	}

	type prp struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	originalInfoStruct := struct {
		SolutionSource string                 `json:"solution_source"`
		Params         map[string]any         `json:"params"`
		ParamsDefine   types.HyperParamDefine `json:"params_define"`
		Property       prp                    `json:"property"`
	}{
		SolutionSource: "snapsolution",
		Params:         params,
		ParamsDefine:   hyp,
		Property: prp{
			ID:      soltuion.ID,
			Name:    soltuion.Name,
			Version: soltuion.Version,
		},
	}

	originalString, err := json.Marshal(originalInfoStruct)
	if err != nil {
		fmt.Println("Error marshaling struct:", err)
		return nil
	}

	var hyperParams []*types.HyperParam
	for k, v := range params {
		hyperParams = append(hyperParams, &types.HyperParam{
			SnapdetKey: k,
			Value:      v,
		})
	}

	return &types.Algorithm{
		ID:               soltuion.ID,
		Name:             soltuion.Name,
		BackboneName:     soltuion.ModelInfo.BackboneName,
		SuperName:        soltuion.ModelInfo.SuperName,
		Description:      soltuion.Description,
		HyperParam:       hyperParams,
		HyperParamDefine: &hyp,
		Properties:       properties,
		OriginalInfo:     string(originalString),
	}
}
