package hdlr

import (
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/ctx"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/ctx/errors"
	autoLearnMgr "go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/mgr/controller/autolearn"
	client "go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/outer-client"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/utils"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	aisConst "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
)

func (m *Mgr) RecommendSamplerBuild(c *gin.Context) {
	gc := ginlib.NewGinContext(c)

	var req types.SnapSamplerBuildRequest
	if err := gc.GetRequestJSONBody(&req); err != nil {
		log.WithError(err).Error("failed to parser request body")
		ctx.Error(c, errors.ErrorInvalidParams)
		return
	}

	for _, sdsPath := range req.SDSPaths {
		if err := utils.CheckOssPathExist(m.OssClient, sdsPath); err != nil {
			log.WithError(err).Error("failed to check SDS URI")
			ctx.Error(c, errors.ErrorInvalidParams)
			return
		}
	}

	snapSampler, _, err := client.BuildRecommendSnapSampler(&client.RecommendSamplerBuildReq{
		TrainType:               req.Type,
		SnapSamplerBuildRequest: &req,
	})
	if err != nil {
		log.WithError(err).Error("failed to BuildRecommendSnapSampler")
		ctx.Error(c, err)
		return
	}

	ctx.Success(c, &snapSampler)
}

func (m *Mgr) RecommendSamplerUpdate(c *gin.Context) {
	gc := ginlib.NewGinContext(c)

	var req types.SnapSamplerUpdateRequest
	if err := gc.GetRequestJSONBody(&req); err != nil {
		log.WithError(err).Error("failed to parser request body")
		ctx.Error(c, errors.ErrorInvalidParams)
		return
	}

	snapSampler, _, err := client.UpdateRecommendSnapSampler(&client.RecommendSamplerUpdateReq{
		TrainType:                req.Type,
		SnapSamplerUpdateRequest: &req,
	})
	if err != nil {
		log.WithError(err).Error("failed to BuildRecommendSnapSampler")
		ctx.Error(c, err)
		return
	}

	ctx.Success(c, &snapSampler)
}

// @Summary 获取数据源的分类类别名
// @tags recommendation
// @Produce application/json
// @Description 获取数据集、训验对、SDS中的分类类别
// @Param autoLearnCreate body types.DatasetAttributeRequest true "获取类别"
// @Success 200 {object} types.DataSourceAttributeResp "请求成功"
// @Failure 400 {object} ctx.errorResponse "参数非法"
// @Failure 401 {object} ctx.errorResponse "认证失败"
// @Failure 404 {object} ctx.errorResponse "资源不存在"
// @Router /dataset-attribute [post]
func (m *Mgr) GetDataSourceAttribute(c *gin.Context) {
	gc := ginlib.NewGinContext(c)

	var req types.DatasetAttributeRequest
	if err := gc.GetRequestJSONBody(&req); err != nil {
		log.WithError(err).Error("failed to parser request body")
		ctx.Error(c, errors.ErrorInvalidParams)
		return
	}

	if req.Dataset == nil {
		log.Error("dataset is nil")
		ctx.Error(c, errors.ErrorInvalidParams)
		return
	}

	var wrapDatasets []*types.Dataset
	if err := m.Controller.WrapDataset(gc, req.Dataset, &wrapDatasets); err != nil {
		log.WithError(err).Error("failed to wrap datasets info")
		ctx.Error(c, err)
		return
	}

	// 训验对需要返回训练集和验证集 [[],[]]
	respData := types.DataSourceAttributeResp{}
	for i := range wrapDatasets {
		attributes, httpCode, err := client.GetSDSFileClass(aisConst.AppTypeClassification, wrapDatasets[i].SDSPath)
		if err != nil {
			log.WithError(err).Error("failed to get sds file attributes")
			if httpCode == 400 {
				ctx.Error(c, errors.ErrorNotClassificationDataset)
				return
			}
			ctx.Error(c, err)
			return
		}
		respData.Attributes = append(respData.Attributes, attributes)
	}
	respData.AttributeType = autoLearnMgr.GetDatasetAttributeType(respData.Attributes)

	ctx.Success(c, &respData)
}

func (m *Mgr) GetDataSourcesAttribute(c *gin.Context) {
	gc := ginlib.NewGinContext(c)

	var req types.DatasetsAttributeRequest
	if err := gc.GetRequestJSONBody(&req); err != nil {
		log.WithError(err).Error("failed to parser request body")
		ctx.Error(c, errors.ErrorInvalidParams)
		return
	}

	if req.Datasets == nil {
		log.Error("datasets is nil")
		ctx.Error(c, errors.ErrorInvalidParams)
		return
	}

	var wrapDatasets []*types.Dataset
	for _, ds := range req.Datasets {
		if err := m.Controller.WrapDataset(gc, ds, &wrapDatasets); err != nil {
			log.WithError(err).Error("failed to wrap datasets info")
			ctx.Error(c, err)
			return
		}
	}

	// 训验对需要返回训练集和验证集 [[],[]]
	respData := types.DataSourceAttributeResp{}
	for i := range wrapDatasets {
		attributes, httpCode, err := client.GetSDSFileClass(aisConst.AppTypeClassification, wrapDatasets[i].SDSPath)
		if err != nil {
			log.WithError(err).Error("failed to get sds file attributes")
			if httpCode == 400 {
				ctx.Error(c, errors.ErrorNotClassificationDataset)
				return
			}
			ctx.Error(c, err)
			return
		}
		respData.Attributes = append(respData.Attributes, attributes)
	}
	respData.AttributeType = autoLearnMgr.GetDatasetAttributeType(respData.Attributes)

	ctx.Success(c, &respData)
}
