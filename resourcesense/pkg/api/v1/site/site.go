package site

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/controller/site"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types/ctx"
	siteTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types/site"
)

type ListSitesResponse struct {
	Total int64             `json:"total"`
	Items []*siteTypes.Site `json:"items"`
}

func ListSites(c *gin.Context) {
	gc := ginlib.NewGinContext(c)
	total, sites, err := site.ListSites(gc)
	if err != nil {
		log.WithError(err).Error("failed to InsertOrUpdateSite")
		ctx.Error(c, err)
		return
	}
	var center *siteTypes.Site
	for i := range sites {
		if sites[i].IsCenter {
			center = sites[i]
		}
		if strings.HasPrefix(sites[i].Domain, "http://") || strings.HasPrefix(sites[i].Domain, "https://") {
			continue
		}
		sites[i].Domain = fmt.Sprintf("https://%s", sites[i].Domain)
		sites[i].AdminDomain = fmt.Sprintf("https://%s", sites[i].AdminDomain)
	}
	if center != nil {
		sites = append([]*siteTypes.Site{
			{
				Name:        "all",
				DisplayName: "全部",
				Domain:      center.Domain,
				AdminDomain: center.AdminDomain,
				IsCenter:    center.IsCenter,
				CreatedAt:   center.CreatedAt,
				LastSeen:    center.LastSeen,
				IsAll:       true,
			},
		}, sites...)
		total++
	}

	ctx.Success(c, &ListSitesResponse{
		Total: total,
		Items: sites,
	})
}

// 数据处理列表
// https://ais.brainpp.cn/aapi/datahub.ais.io/api/v1/clean/tasks?page=1&pageSize=40&onlyme=false&sortBy=DESC&order=createdAt&name=
// 数据集列表
// https://ais.brainpp.cn/aapi/datahub.ais.io/api/v1/datasets?page=1&pageSize=6&sortBy=createdAt&order=desc&resourceType=Project&onlyme=
// 训验对列表
// https://ais.brainpp.cn/aapi/datahub.ais.io/api/v1/pairs?page=1&pageSize=6&sortBy=createdAt&order=desc&onlyme=&name=
// 智能标注列表
// https://ais.brainpp.cn/aapi/ailabel.ais.io/api/v1/tasks?page=1&pageSize=20&sortBy=createdAt&order=desc&labelingState=&onlyMe=false
// 快速标注列表
// https://ais.brainpp.cn/aapi/basiclabel.ais.io/api/v1/tasks?page=1&pageSize=20&sortBy=createdAt&order=desc&workMode=Simple&onlyMe=false
// 多人标注列表
// https://ais.brainpp.cn/aapi/basiclabel.ais.io/api/v1/tasks?page=1&pageSize=20&sortBy=createdAt&order=desc&category=All&workMode=Cooperation&onlyMe=false
// 算法管理-平台算法 (C)
// https://ais.brainpp.cn/aapi/autolearn.ais.io/v1/autolearns-solutions
// 算法管理-自定义算法
// https://ais.brainpp.cn/aapi/codehub.ais.io/v1/codebases?codeType=Solution&taskType=Solution&appType=Detection&appType=Classification&appType=KPS&appType=Regression&appType=Segmentation&page=1&pageSize=10&sortBy=updatedAt&order=desc&onlyMe=false&searchKey=
// 自动学习列表
// https://ais.brainpp.cn/aapi/autolearn.ais.io/v1/autolearns?page=1&pageSize=20&sortBy=updatedAt&order=desc&onlyMe=false&autoLearnName=
// 模型管理列表
// https://ais.brainpp.cn/aapi/modelhub.ais.io/v1/models?page=1&pageSize=10&isOwner=false&showAll=true&name=
// 评测列表
// https://ais.brainpp.cn/aapi/evalhub.ais.io/api/v1/evaluations?page=1&pageSize=20&sortBy=createdAt&order=desc&collapseMode=Fold&isOwner=false&name=&jobName=&modelName=&datasetName=&createdBy=
// 评测对比列表
// https://ais.brainpp.cn/aapi/evalhub.ais.io/api/v1/evaluations?page=1&pageSize=40&jobStatus=Successed&sortBy=updatedAt&order=desc&collapseMode=UnFold&appType=Detection&name=&modelName=&datasetName=&createdBy=
// 推理服务列表
// https://ais.brainpp.cn/aapi/inference.ais.io/v1/inferences?page=1&pageSize=40&onlyMe=false&name=
// 算法卡片列表
// https://ais.brainpp.cn/aapi/algohub.ais.io/api/v1/algocards?page=1&pageSize=20&sortBy=updatedAt&order=desc&onlyMe=false&name=
// 算法模版-平台算法仓模板
// https://ais.brainpp.cn/aapi/codehub.ais.io/v1/codebases?codeType=AlgoCabin&page=1&pageSize=10000&sortBy=codeName&order=asc&searchKey=&onlyMe=false&onlyUnbanned=true
// 算法模版-自定义算法仓模版
// https://ais.brainpp.cn/aapi/codehub.ais.io/v1/codebases?codeType=AlgoCabin&taskType=AlgoCabin&page=1&pageSize=20&sortBy=updatedAt&order=desc&onlyMe=false&searchKey=v
// 算法仓列表
// https://ais.brainpp.cn/aapi/algohub.ais.io/api/v1/algocabins?pageSize=12&page=1&order=desc&sortBy=updatedAt&onlyMe=false&name=

func ListSiteProxy(c *gin.Context) {
	gc := ginlib.NewGinContext(c)
	client := site.NewClient(nil)
	log.Infof("fuck page: %+v", gc.GetPagination())

	_, sites, err := site.ListSites(gc)
	if err != nil {
		log.WithError(err).Error("failed to ListSites")
		ctx.Error(c, err)
		return
	}
	response, err := client.Proxy(gc, sites)
	if err != nil {
		log.WithError(err).Error("failed to Proxy")
		ctx.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, response)
}

func SitesClusterQuota(c *gin.Context) {
	gc := ginlib.NewGinContext(c)
	client := site.NewClient(nil)
	_, sites, err := site.ListSites(gc)
	if err != nil {
		log.WithError(err).Error("failed to ListSites")
		ctx.Error(c, err)
		return
	}

	var response *site.SitesClusterQuota
	if gc.GetAuthTenantID() != "" {
		response, err = client.SitesTenantQuota(gc, sites)
	} else {
		response, err = client.SitesClusterQuota(gc, sites)
	}
	if err != nil {
		log.WithError(err).Error("failed to Proxy")
		ctx.Error(c, err)
		return
	}
	ctx.Success(c, response)
}

func SitesClusterAPU(c *gin.Context) {
	gc := ginlib.NewGinContext(c)
	client := site.NewClient(nil)
	_, sites, err := site.ListSites(gc)
	if err != nil {
		log.WithError(err).Error("failed to ListSites")
		ctx.Error(c, err)
		return
	}

	response, err := client.SitesClusterAPU(gc, sites)
	if err != nil {
		log.WithError(err).Error("failed to Proxy")
		ctx.Error(c, err)
		return
	}
	ctx.Success(c, response)
}

func ReportSiteStatus(c *gin.Context) {
	gc := ginlib.NewGinContext(c)

	var req site.ReportPayload
	if err := gc.BindJSON(&req); err != nil {
		log.WithError(err).Error("failed to BindJSON")
		ctx.Error(c, err)
		return
	}
	// 补充一些创建 vs / 创建相关配置的东西
	if err := site.InsertOrUpdateSite(gc, &req); err != nil {
		log.WithError(err).Error("failed to InsertOrUpdateSite")
		ctx.Error(c, err)
		return
	}

	go func() {
		if err := site.OnSiteJoin(context.Background(), req.SiteInfo); err != nil {
			log.WithError(err).Errorf("failed on site join: %+v", req.SiteInfo)
		}
	}()
	ctx.Success(c, struct{}{})
}
