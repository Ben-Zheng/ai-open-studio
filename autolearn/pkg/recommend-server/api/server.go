package api

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"

	tps "go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/recommend-server/types"
	aisConst "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
)

type Mgr struct{}

type RecommendServer struct {
	*gin.Engine
}

func NewRecommendServer() (*RecommendServer, error) {
	ginServer := gin.New()
	mgr := &Mgr{}
	ginServer.GET("/snapdet/dataset/summarize", mgr.ListDatasetCategories)
	ginServer.POST("/snapdet/recommend/sampler", mgr.BuildRecommendSnapSampler)
	ginServer.PUT("/snapdet/recommend/sampler", mgr.UpdateRecommendSnapSampler)
	return &RecommendServer{
		ginServer,
	}, nil
}

func loadSnapSamplerInput(req *tps.SnapSamplerBuildRequest) *types.SnapSamplerInput {
	return &types.SnapSamplerInput{
		Method:   req.Method,
		Task:     tps.TaskTypeMap[req.Type],
		SDSPaths: req.SDSPaths,
	}
}

func loadSnapSamplerAndWeight(req *tps.SnapSamplerUpdateRequest) (*types.SnapSampler, *types.SnapSamplerWeight) {
	snapSampler := LoadSnapSampler(req.SnapSampler)
	return snapSampler, &types.SnapSamplerWeight{
		Weights:          req.Weights,
		SnapSamplerInput: snapSampler.SnapSamplerInput,
	}
}

func LoadSnapSampler(snapSampler *tps.SnapSampler) *types.SnapSampler {
	var atomSamplers []types.AtomSampler
	for _, atomSampler := range snapSampler.AtomSamplers {
		atomSamplers = append(atomSamplers, types.AtomSampler{
			Name: atomSampler.Name,
			Num:  atomSampler.Num,
			StatisticInfo: types.StatisticInfo{
				DatasetInfo: atomSampler.StatisticInfo.DatasetInfo,
				ClassInfo:   atomSampler.StatisticInfo.ClassInfo,
			},
		})
	}

	return &types.SnapSampler{
		SnapSamplerWeight: types.SnapSamplerWeight{
			SnapSamplerInput: types.SnapSamplerInput{
				SDSPaths: snapSampler.SDSPaths,
				Method:   snapSampler.Method,
				Task:     snapSampler.Task,
			},
			Weights: snapSampler.Weights,
		},
		AtomSamplers: atomSamplers,
		StatisticInfo: types.StatisticInfo{
			DatasetInfo: snapSampler.StatisticInfo.DatasetInfo,
			ClassInfo:   snapSampler.StatisticInfo.ClassInfo,
		},
	}
}

func mapSnapSampler(snapSampler *types.SnapSampler) *tps.SnapSampler {
	var atomSamplers []tps.AtomSampler
	for _, atomSampler := range snapSampler.AtomSamplers {
		atomSamplers = append(atomSamplers, tps.AtomSampler{
			Name: atomSampler.Name,
			Num:  atomSampler.Num,
			StatisticInfo: tps.StatisticInfo{
				DatasetInfo: atomSampler.StatisticInfo.DatasetInfo,
				ClassInfo:   atomSampler.StatisticInfo.ClassInfo,
			},
		})
	}

	return &tps.SnapSampler{
		SnapSamplerWeight: tps.SnapSamplerWeight{
			SnapSamplerInput: tps.SnapSamplerInput{
				SDSPaths: snapSampler.SDSPaths,
				Method:   snapSampler.Method,
				Task:     snapSampler.Task,
			},
			Weights: snapSampler.Weights,
		},
		AtomSamplers: atomSamplers,
		StatisticInfo: tps.StatisticInfo{
			DatasetInfo: snapSampler.StatisticInfo.DatasetInfo,
			ClassInfo:   snapSampler.StatisticInfo.ClassInfo,
		},
	}
}

func (mgr *Mgr) BuildRecommendSnapSampler(ctx *gin.Context) {
	var req tps.SnapSamplerBuildRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		log.WithError(err).Error("failed to parser request body")
		ctx.JSON(http.StatusInternalServerError, nil)
		return
	}

	if req.Type == aisConst.AppTypeOCR {
		ctx.JSON(http.StatusOK, mapSnapSampler(&types.SnapSampler{
			SnapSamplerWeight: types.SnapSamplerWeight{
				SnapSamplerInput: types.SnapSamplerInput{
					Task: tps.TaskTypeMap[aisConst.AppTypeOCR],
				},
			},
		}))
		return
	}

	data, err := json.Marshal(loadSnapSamplerInput(&req))
	if err != nil {
		log.WithError(err).Error("failed to Marshal")
		ctx.JSON(http.StatusInternalServerError, nil)
		return
	}

	dir, err := os.MkdirTemp("", "data-sampler")
	if err != nil {
		log.WithError(err).Error("create tmp dir failed")
		ctx.JSON(http.StatusInternalServerError, nil)
		return
	}

	outputFilePath := filepath.Join(dir, "output.json")
	inputFilePath := filepath.Join(dir, "input.json")
	if err := os.WriteFile(inputFilePath, data, os.ModePerm); err != nil {
		log.WithError(err).Error("failed to WriteFile")
		ctx.JSON(http.StatusInternalServerError, nil)
		return
	}

	cmd := exec.Command(
		"snapsampler", "build", "--input", inputFilePath, "--output", outputFilePath,
	)

	cmd.Env = os.Environ()
	log.Infof("cmd: %v", cmd.Args)
	log.Info(string(data))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		log.WithError(err).Errorf("cmd %v start failed", cmd.Args)
		ctx.JSON(http.StatusInternalServerError, nil)
		return
	}

	if err := cmd.Wait(); err != nil {
		log.WithError(err).Errorf("snapsampler process %d exit with err/signal: %v\n", cmd.Process.Pid, err)
		ctx.JSON(http.StatusInternalServerError, nil)
		return
	}

	file, err := os.ReadFile(outputFilePath)
	if err != nil {
		log.WithError(err).Errorf("open output.json: %s file failed", outputFilePath)
		ctx.JSON(http.StatusBadRequest, nil)
		return
	}
	var snapSampler types.SnapSampler
	err = json.Unmarshal(file, &snapSampler)
	if err != nil {
		log.WithError(err).Errorf("decode output.json: %s file failed", outputFilePath)
		ctx.JSON(http.StatusInternalServerError, nil)
		return
	}

	ctx.JSON(http.StatusOK, mapSnapSampler(&snapSampler))
}

func (mgr *Mgr) UpdateRecommendSnapSampler(ctx *gin.Context) {
	var req tps.SnapSamplerUpdateRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		log.WithError(err).Error("failed to parser request body")
		ctx.JSON(http.StatusInternalServerError, nil)
		return
	}

	if req.SnapSampler != nil {
		if req.SnapSampler.Task == tps.TaskTypeOCR {
			ctx.JSON(http.StatusOK, mapSnapSampler(&types.SnapSampler{}))
			return
		}
	}

	sampler, weight := loadSnapSamplerAndWeight(&req)

	dataSampler, err := json.Marshal(sampler)
	if err != nil {
		log.WithError(err).Error("failed to Marshal Sampler")
		ctx.JSON(http.StatusInternalServerError, nil)
		return
	}

	dataWeight, err := json.Marshal(weight)
	if err != nil {
		log.WithError(err).Error("failed to Marshal Weight")
		ctx.JSON(http.StatusInternalServerError, nil)
		return
	}

	log.Info(string(dataSampler))
	log.Info(string(dataWeight))

	dir, err := os.MkdirTemp("", "data-sampler")
	if err != nil {
		log.WithError(err).Error("create tmp dir failed")
		ctx.JSON(http.StatusInternalServerError, nil)
		return
	}

	outputFilePath := filepath.Join(dir, "output.json")
	samplerFilePath := filepath.Join(dir, "sampler.json")
	if err := os.WriteFile(samplerFilePath, dataSampler, os.ModePerm); err != nil {
		log.WithError(err).Error("failed to WriteFile sampler")
		ctx.JSON(http.StatusInternalServerError, nil)
		return
	}

	weightFilePath := filepath.Join(dir, "weight.json")
	if err := os.WriteFile(weightFilePath, dataWeight, os.ModePerm); err != nil {
		log.WithError(err).Error("failed to WriteFile weight")
		ctx.JSON(http.StatusInternalServerError, nil)
		return
	}

	cmd := exec.Command(
		"snapsampler", "update", "--input", samplerFilePath, "--weight", weightFilePath, "--output", outputFilePath,
	)

	cmd.Env = os.Environ()
	log.Infof("cmd: %v", cmd.Args)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		log.WithError(err).Errorf("cmd %v start failed", cmd.Args)
		ctx.JSON(http.StatusInternalServerError, nil)
		return
	}

	if err := cmd.Wait(); err != nil {
		log.WithError(err).Errorf("snapsampler process %d exit with err/signal: %v\n", cmd.Process.Pid, err)
		ctx.JSON(http.StatusInternalServerError, nil)
		return
	}

	file, err := os.ReadFile(outputFilePath)
	if err != nil {
		log.WithError(err).Errorf("open output.json: %s file failed", outputFilePath)
		ctx.JSON(http.StatusBadRequest, nil)
		return
	}
	var snapSampler types.SnapSampler
	err = json.Unmarshal(file, &snapSampler)
	if err != nil {
		log.WithError(err).Errorf("decode output.json: %s file failed", outputFilePath)
		ctx.JSON(http.StatusInternalServerError, nil)
		return
	}

	ctx.JSON(http.StatusOK, mapSnapSampler(&snapSampler))
}

func (mgr *Mgr) ListDatasetCategories(ctx *gin.Context) {
	valSDS, ok := ctx.GetQuery(types.QUERY_VAL_SDS)
	if !ok || valSDS == "" {
		log.Error("request params gpu num is invalid")
		ctx.JSON(http.StatusInternalServerError, nil)
	}

	dir, err := os.MkdirTemp("", "snapclf-summarize")
	if err != nil {
		log.WithError(err).Error("create tmp dir failed")
	}
	filePath := filepath.Join(dir, "output.json")
	cmd := exec.Command(
		"snapclf", "data", "summarize",
		valSDS,
		"--file-output", filePath,
	)

	cmd.Env = os.Environ()

	log.Infof("cmd: %v", cmd.Args)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		log.WithError(err).Errorf("cmd %v start failed", cmd.Args)
		ctx.JSON(http.StatusInternalServerError, nil)
		return
	}

	if err := cmd.Wait(); err != nil {
		log.WithError(err).Errorf("snapclf summarize process %d exit with err/signal: %v\n", cmd.Process.Pid, err)
		ctx.JSON(http.StatusInternalServerError, nil)
		return
	}

	file, err := os.ReadFile(filePath)
	if err != nil {
		log.WithError(err).Errorf("open output.json: %s file failed", filePath)
		ctx.JSON(http.StatusBadRequest, nil)
		return
	}
	var categories types.CategorySummarize
	err = json.Unmarshal(file, &categories)
	if err != nil {
		log.WithError(err).Errorf("decode output.json: %s file failed", filePath)
		ctx.JSON(http.StatusInternalServerError, nil)
		return
	}
	ctx.JSON(http.StatusOK, categories)
}
