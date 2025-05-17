package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/config"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/ctx/errors"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/types"
	aisConst "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
)

const (
	// 请求recommend-server接口超时时间，单位s
	recommendSnapSamplerBuildRequestTimeout  = 60 * 8
	recommendSnapSamplerUpdateRequestTimeout = 60 * 1

	// 获取推荐算法
	getRecommendSnapSamplerPath = "snapdet/recommend/sampler"
)

type RecommendSamplerBuildReq struct {
	TrainType               aisConst.AppType               `json:"trainType"`
	SnapSamplerBuildRequest *types.SnapSamplerBuildRequest `json:"snapSamplerBuildRequest"`
}
type RecommendSamplerUpdateReq struct {
	TrainType                aisConst.AppType                `json:"trainType"`
	SnapSamplerUpdateRequest *types.SnapSamplerUpdateRequest `json:"snapSamplerUpdateRequest"`
}

// BuildRecommendSnapSampler 创建数据采样器
func BuildRecommendSnapSampler(reqParams *RecommendSamplerBuildReq) (*types.SnapSampler, int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), recommendSnapSamplerBuildRequestTimeout*time.Second)
	defer cancel()

	serverAddr := config.Profile.AisEndPoint.AutoLearnSampler
	bodyBytes, err := json.Marshal(reqParams.SnapSamplerBuildRequest)
	if err != nil {
		log.Error("failed failed to marshal creatBody")
		return nil, 0, err
	}

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/%s", serverAddr, getRecommendSnapSamplerPath), bytes.NewReader(bodyBytes))
	if err != nil {
		log.WithError(err).Error("failed to create new recommendServer request")
		return nil, 0, err
	}

	log.Infof("Get recommend data sampler uri: %s", req.URL.String())

	resp, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		log.WithError(err).Error("failed to request recommend server")
		return nil, 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.WithError(err).Error("failed to read response body")
		return nil, resp.StatusCode, err
	}

	var snapSampler types.SnapSampler

	if err := json.Unmarshal(body, &snapSampler); err != nil {
		log.WithError(err).Error("failed to unmarshal body")
		return nil, resp.StatusCode, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, errors.ErrorBuildSnapSampler
	}

	return &snapSampler, resp.StatusCode, nil
}

// BuildRecommendSampler 创建数据采样器
func UpdateRecommendSnapSampler(reqParams *RecommendSamplerUpdateReq) (*types.SnapSampler, int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), recommendSnapSamplerUpdateRequestTimeout*time.Second)
	defer cancel()

	serverAddr := config.Profile.AisEndPoint.AutoLearnSampler
	bodyBytes, err := json.Marshal(reqParams.SnapSamplerUpdateRequest)
	if err != nil {
		log.Error("failed failed to marshal creatBody")
		return nil, 0, err
	}

	req, err := http.NewRequest("PUT", fmt.Sprintf("%s/%s", serverAddr, getRecommendSnapSamplerPath), bytes.NewReader(bodyBytes))
	if err != nil {
		log.WithError(err).Error("failed to create new recommendServer request")
		return nil, 0, err
	}

	log.Infof("Get recommend data sampler uri: %s", req.URL.String())

	resp, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		log.WithError(err).Error("failed to request recommend server")
		return nil, 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.WithError(err).Error("failed to read response body")
		return nil, resp.StatusCode, err
	}

	var snapSampler types.SnapSampler

	if err := json.Unmarshal(body, &snapSampler); err != nil {
		log.WithError(err).Error("failed to unmarshal body")
		return nil, resp.StatusCode, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, errors.ErrorBuildSnapSampler
	}

	return &snapSampler, resp.StatusCode, nil
}
