package algoresource

import (
	"fmt"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/controller/quota"
	"math/rand"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson/primitive"

	algohubV1 "go.megvii-inc.com/brain/brainpp/projects/aiservice/algohub/pkg/api/v1"
	algocardV1 "go.megvii-inc.com/brain/brainpp/projects/aiservice/algohub/pkg/api/v1/algocard"
	algohubTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/algohub/pkg/types"
	authv1 "go.megvii-inc.com/brain/brainpp/projects/aiservice/auth/pkg/api/v1"
	authTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/auth/pkg/app/types"
	codehubV1 "go.megvii-inc.com/brain/brainpp/projects/aiservice/codehub/pkg/api/v1"
	codehubType "go.megvii-inc.com/brain/brainpp/projects/aiservice/codehub/pkg/types"
	modelhubApi "go.megvii-inc.com/brain/brainpp/projects/aiservice/modelhub/pkg/api"
	modelhubCtrl "go.megvii-inc.com/brain/brainpp/projects/aiservice/modelhub/pkg/controller/model"
	modelhubTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/modelhub/pkg/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/features"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	aisTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/consts"
	algoresourceCtrl "go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/controller/algoresource"
	algoresourceType "go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types/algoresource"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types/ctx"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types/ctx/errors"
)

type listResponse struct {
	Total int64                          `json:"total"`
	Items []*algoresourceType.TenantAlgo `json:"items"`
}

func List(c *gin.Context) {
	gc := ginlib.NewGinContext(c)
	tenantName := gc.Query("tenantName")
	if !features.IsPersonalWorkspaceEnabled() {
		gc.Set("tenantType", "team")
	}
	// 运营端 admin token 检测
	if strings.Contains(gc.Request.Host, "admin") {
		aisToken := gc.GetAuthToken()
		selfUid := gc.GetUserID()
		err := quota.CheckIsAdminToken(gc, aisToken, selfUid)
		if err != nil {
			log.WithError(err).Error("check admin token failed")
			ctx.Error(c, err)
			return
		}
	}

	total, tas, err := algoresourceCtrl.List(gc, tenantName, gc.GetPagination(), gc.GetSort())
	if err != nil {
		log.WithError(err).Error("failed to List")
		ctx.Error(c, errors.ErrInternal)
		return
	}

	ctx.Success(c, listResponse{
		Total: total,
		Items: tas,
	})
}

func Get(c *gin.Context) {
	gc := ginlib.NewGinContext(c)
	tenantID := gc.Param("tenantID")
	// 运营端 admin token 检测
	if strings.Contains(gc.Request.Host, "admin") {
		aisToken := gc.GetAuthToken()
		selfUid := gc.GetUserID()
		err := quota.CheckIsAdminToken(gc, aisToken, selfUid)
		if err != nil {
			log.WithError(err).Error("check admin token failed")
			ctx.Error(c, err)
			return
		}
	}

	ta, err := algoresourceCtrl.GetByTenantID(gc, tenantID)
	if err != nil {
		log.WithError(err).Error("failed to List")
		ctx.Error(c, err)
		return
	}

	ctx.Success(c, ta)
}

type updateRequest struct {
	AlgoType algoresourceType.AlgoType   `json:"algoType"`
	Records  []*algoresourceType.Content `json:"records"`
}

func Update(c *gin.Context) {
	gc := ginlib.NewGinContext(c)
	tenantID := gc.Param("tenantID")

	var ur updateRequest
	if err := gc.BindJSON(&ur); err != nil {
		log.WithError(err).Error("failed to BindJSON")
		ctx.Error(c, err)
		return
	}
	// 运营端 admin token 检测
	if strings.Contains(gc.Request.Host, "admin") {
		aisToken := gc.GetAuthToken()
		selfUid := gc.GetUserID()
		err := quota.CheckIsAdminToken(gc, aisToken, selfUid)
		if err != nil {
			log.WithError(err).Error("check admin token failed")
			ctx.Error(c, err)
			return
		}
	}

	ta, err := algoresourceCtrl.GetByTenantID(gc, tenantID)
	if err != nil {
		log.WithError(err).Error("failed to List")
		ctx.Error(c, err)
		return
	}
	if err := update(gc, &ur, ta); err != nil {
		log.WithError(err).Error("failed to update")
		ctx.Error(c, err)
		return
	}

	ctx.Success(c, nil)
}

func update(gc *ginlib.GinContext, ur *updateRequest, ta *algoresourceType.TenantAlgo) error {
	contents := ur.Records
	addContents, subContents := getDiffContent(ur, ta)

	authClient := authv1.NewClientWithAuthorazation(consts.ConfigMap.AuthServer,
		consts.ConfigMap.MultiSiteConfig.CenterAK, consts.ConfigMap.MultiSiteConfig.CenterSK)
	t, err := authClient.GetTenant(gc, ta.TenantID)
	if err != nil {
		log.WithError(err).Error("failed to GetTenant")
		return err
	}
	ps, err := authClient.ListProjectByTenant(gc, ta.TenantID)
	if err != nil {
		log.WithError(err).Error("failed to ListProjectByTenant")
		return err
	}

	taa := &algoresourceType.TenantAlgoAudit{
		ID:         primitive.NewObjectID(),
		TenantID:   ta.TenantID,
		TenantName: t.Name,
		AlgoType:   ur.AlgoType,
		Records:    contents,
		CreatorID:  gc.GetUserID(),
		Creator:    gc.GetUserName(),
		CreatedAt:  time.Now().Unix(),
	}

	if err := algoresourceCtrl.Update(gc, ta.ID, ur.AlgoType, contents); err != nil {
		log.WithError(err).Error("failed to Update")
		return err
	}

	if err := algoresourceCtrl.CreateAudit(gc, taa); err != nil {
		log.WithError(err).Error("failed to CreateAudit")
		return err
	}

	if ur.AlgoType == algoresourceType.AlgoCard {
		if err := distributeAlgoCard(gc, ps, contents); err != nil {
			log.WithError(err).Error("failed to distributeAlgoCard")
			return err
		}
	} else {
		if err := distributePlatform(gc, ps, addContents, subContents, t); err != nil {
			log.WithError(err).Error("failed to distributePlatform")
			return err
		}
	}
	return nil
}

func getDiffContent(ur *updateRequest, ta *algoresourceType.TenantAlgo) ([]*algoresourceType.Content, []*algoresourceType.Content) {
	var newIDMap = map[string]bool{}
	var newContents []*algoresourceType.Content
	for _, content := range ur.Records {
		if _, ok := newIDMap[content.ID]; !ok {
			newIDMap[content.ID] = true
			newContents = append(newContents, content)
		}
	}

	oldContents := ta.Platforms
	if ur.AlgoType == algoresourceType.AlgoCard {
		oldContents = ta.AlgoCards
	}
	var oldIDMap = map[string]bool{}
	for _, content := range oldContents {
		oldIDMap[content.ID] = true
	}

	var addContents, subContents []*algoresourceType.Content
	for _, content := range newContents {
		if _, ok := oldIDMap[content.ID]; !ok {
			addContents = append(addContents, content)
		}
	}
	for _, content := range oldContents {
		if _, ok := newIDMap[content.ID]; !ok {
			subContents = append(subContents, content)
		}
	}

	return addContents, subContents
}

func distributeAlgoCard(gc *ginlib.GinContext, ps []*authTypes.Project, contents []*algoresourceType.Content) error {
	for _, content := range contents {
		cd, err := algohubV1.NewClient(consts.ConfigMap.EndPoint.AlgoHub).GetAlgoCard(gc, content.ID, aisTypes.SystemLevel)
		if err != nil {
			log.WithError(err).Error("failed to GetAlgoCard")
			return err
		}
		// 列举组内项目
		for _, p := range ps {
			tpa, err := algoresourceCtrl.GetTenantProjectRecordByID(gc, p.Tenant.ID, p.ID, content.ID)
			if err != nil {
				log.WithError(err).Error("failed to GetTenantProjectRecordByID")
				return err
			}
			if tpa != nil {
				// 已经存在
				continue
			}

			newModelUrls := make([]string, len(cd.RepackerSpec.Dependence.ModelDetails))
			newModelDetails := make([]*algohubTypes.ModelDetail, len(cd.RepackerSpec.Dependence.ModelDetails))
			repackerSpec := &algohubTypes.RepackerSpec{
				Name:            cd.RepackerSpec.Name,
				Description:     cd.RepackerSpec.Description,
				Version:         cd.RepackerSpec.Version,
				Devices:         cd.RepackerSpec.Devices,
				Key:             cd.RepackerSpec.Key,
				RawMeta:         cd.RepackerSpec.RawMeta,
				Schema:          cd.RepackerSpec.Schema,
				RepackerPackage: cd.RepackerSpec.RepackerPackage,
				Dependence: &algohubTypes.Dependence{
					ModelDetails: newModelDetails,
					ModelUrls:    newModelUrls,
					Repackers:    cd.RepackerSpec.Dependence.Repackers,
				},
			}
			for i, model := range cd.RepackerSpec.Dependence.ModelDetails {
				repackerSpec.Dependence.ModelDetails[i] = &algohubTypes.ModelDetail{
					ModelURL:        model.ModelURL,
					ModelID:         model.ModelID,
					RawRevisionID:   model.RawRevisionID,
					RevisionID:      model.RevisionID,
					ModelName:       model.ModelName,
					RawRevisionName: model.RawRevisionName,
					RevisionName:    model.RevisionName,
					RawFileURI:      model.RawFileURI,
					ModelFileURI:    model.ModelFileURI,
					AppType:         model.AppType,
					Device:          model.Device,
					Platform:        model.Platform,
					PlatformDevice:  model.PlatformDevice,
				}
				gc.Set(authTypes.AISProjectHeader, p.ID)
				gc.Set(authTypes.AISTenantHeader, p.Tenant.ID)
				gc.Set(authTypes.AISSubjectNameHeader, "ais")
				gc.Set(authTypes.AISSubjectHeader, "ais")
				modelName := fmt.Sprintf("%s-%s", model.ModelName, randomSixString())
				body := map[string]any{
					"name":     modelName,
					"appType":  model.AppType,
					"fromType": modelhubTypes.ModelFromTypeAlgohub,
					"entities": []*modelhubCtrl.RevisionCreateRequest{
						{
							Name:     model.RawRevisionName,
							AppType:  model.AppType,
							FromType: modelhubTypes.ModelFromTypeAlgohub,
							AlgohubFrom: &modelhubTypes.AlgohubFrom{
								ModelSourceType: modelhubTypes.ModelHubModelSourceType,
								ModelID:         model.ModelID,
								ModelName:       modelName,
								RevisionID:      model.RawRevisionID,
								RevisionName:    model.RawRevisionName,
								ModelPath:       model.RawFileURI,
								Platform:        "",
								Device:          "",
								PlatformDevice:  "",
								Algocard:        &modelhubTypes.Algocard{},
							},
						},
						{
							Name:     model.RevisionName,
							AppType:  model.AppType,
							FromType: modelhubTypes.ModelFromTypeAlgohub,
							AlgohubFrom: &modelhubTypes.AlgohubFrom{
								ModelSourceType: modelhubTypes.ModelHubModelSourceType,
								ModelID:         model.ModelID,
								ModelName:       modelName,
								RevisionID:      model.RevisionID,
								RevisionName:    model.RevisionName,
								ModelPath:       model.ModelFileURI,
								Platform:        model.Platform,
								Device:          model.Device,
								PlatformDevice:  model.PlatformDevice,
								Algocard:        &modelhubTypes.Algocard{},
							},
						},
					},
				}
				newModel, err := modelhubApi.NewClient(consts.ConfigMap.EndPoint.Modelhub).CreateSDSModel(gc, body)
				if err != nil {
					log.WithError(err).Error("failed to CreateSDSModel")
					return err
				}

				repackerSpec.Dependence.ModelDetails[i].ModelName = modelName
				repackerSpec.Dependence.ModelDetails[i].ModelID = newModel.ModelID
				repackerSpec.Dependence.ModelDetails[i].RawRevisionID = newModel.Revisions[0].Revision
				for _, convert := range newModel.Revisions[0].GroupedConverts {
					if convert.PlatformDevice != model.PlatformDevice {
						continue
					}
					repackerSpec.Dependence.ModelDetails[i].RevisionID = convert.History[0].Revision
					repackerSpec.Dependence.ModelDetails[i].ModelFileURI = convert.History[0].Meta.FileURI
					oldModelURL := strings.ReplaceAll(model.ModelURL, `"`, `\"`)
					newModelURL := strings.ReplaceAll(convert.History[0].AlgoHubModel, `"`, `\"`)
					repackerSpec.Schema = strings.ReplaceAll(repackerSpec.Schema, oldModelURL, newModelURL)
					for _, mURL := range cd.RepackerSpec.Dependence.ModelUrls {
						if mURL == model.ModelURL {
							newModelUrls[i] = convert.History[0].AlgoHubModel
							break
						}
					}
					repackerSpec.Dependence.ModelDetails[i].ModelURL = convert.History[0].AlgoHubModel
				}
			}
			repackerSpec.Dependence.ModelUrls = newModelUrls
			if err := algohubV1.NewClient(consts.ConfigMap.EndPoint.AlgoHub).CreateAlgoCard(gc, aisTypes.ProjectLevel, "ais", "ais", &algocardV1.Request{
				Name:         content.Name,
				TenantID:     p.Tenant.ID,
				ProjectID:    p.ID,
				BaseCardID:   cd.ID.Hex(),
				SourceLevel:  aisTypes.ProjectLevel,
				SourceType:   string(algohubTypes.SourceTypeAllocated),
				RepackerSpec: repackerSpec,
			}); err != nil {
				log.WithError(err).Error("failed to CreateAlgoCard")
				return err
			}
			tpr := &algoresourceType.TenantProjectAlgo{
				ID:        primitive.NewObjectID(),
				TenantID:  p.Tenant.ID,
				ProjectID: p.ID,
				AlgoType:  algoresourceType.AlgoCard,
				RecordID:  content.ID,
				CreatorID: gc.GetUserID(),
				Creator:   gc.GetUserName(),
				CreatedAt: time.Now().Unix(),
			}
			if err := algoresourceCtrl.CreateTenantProjectRecord(gc, tpr); err != nil {
				log.WithError(err).Error("failed to CreateTenantProjectRecord")
				return err
			}
		}
	}
	return nil
}

func distributePlatform(gc *ginlib.GinContext, ps []*authTypes.Project, addContents, subContents []*algoresourceType.Content, tenant *authTypes.Tenant) error {
	// 取消分配
	for _, subContent := range subContents {
		for _, p := range ps {
			tpa, err := algoresourceCtrl.GetTenantProjectRecordByID(gc, p.Tenant.ID, p.ID, subContent.ID)
			if err != nil {
				log.WithError(err).Error("failed to GetTenantProjectRecordByID")
				return err
			}
			gc.SetSourceLevel(aisTypes.ProjectLevel)
			gc.Set(authTypes.AISProjectHeader, p.ID)
			gc.Set(authTypes.AISTenantHeader, p.Tenant.ID)
			if err := codehubV1.NewClient(consts.ConfigMap.EndPoint.CodeHub).DeleteCodebase(gc, tpa.NewRecordID); err != nil {
				log.WithError(err).Error("failed to DeleteCodebase")
				return err
			}
			if err := algoresourceCtrl.DeleteDistributedRecord(gc, p.Tenant.ID, p.ID, string(algoresourceType.Platform), subContent.ID); err != nil {
				log.WithError(err).Error("failed to DeleteDistributedRecord")
				return err
			}
		}
	}
	// 新增分配的
	for _, addContent := range addContents {
		gc.SetSourceLevel(aisTypes.ProjectLevel)
		gc.Set(authTypes.AISProjectHeader, "")
		gc.Set(authTypes.AISTenantHeader, aisTypes.SystemLevelSourceTenantID)
		cb, err := codehubV1.NewClient(consts.ConfigMap.EndPoint.CodeHub).GetCodebase(gc, addContent.ID, "", aisTypes.SystemLevel)
		if err != nil {
			log.WithError(err).Error("failed to GetCodebase")
			return err
		}
		for _, p := range ps {
			projectCodebase, err := codehubV1.NewClient(consts.ConfigMap.EndPoint.CodeHub).Create(gc, aisTypes.ProjectLevel, &codehubType.Codebase{
				TenantID:      p.Tenant.ID,
				ProjectID:     p.ID,
				CodeName:      cb.CodeName,
				CodeType:      cb.CodeType,
				Desc:          cb.Desc,
				Tags:          cb.Tags,
				CreatedBy:     tenant.Owner.ID,
				CreatedByName: tenant.Owner.Name,
				CreatedAt:     time.Now().Unix(),
				UpdatedAt:     time.Now().Unix(),
				Revisions:     cb.Revisions,
				Source:        cb.Source,
				ParentCodeID:  cb.CodeID,
			})
			if err != nil {
				log.WithError(err).Error("failed to Create")
				return err
			}
			tpr := &algoresourceType.TenantProjectAlgo{
				ID:          primitive.NewObjectID(),
				TenantID:    p.Tenant.ID,
				ProjectID:   p.ID,
				AlgoType:    algoresourceType.Platform,
				RecordID:    addContent.ID,
				NewRecordID: projectCodebase.CodeID,
				CreatorID:   tenant.Owner.ID,
				Creator:     tenant.Owner.Name,
				CreatedAt:   time.Now().Unix(),
			}
			if err := algoresourceCtrl.CreateTenantProjectRecord(gc, tpr); err != nil {
				log.WithError(err).Error("failed to CreateTenantProjectRecord")
				return err
			}
		}
	}
	return nil
}

type listAuditResponse struct {
	Total int64                               `json:"total"`
	Items []*algoresourceType.TenantAlgoAudit `json:"items"`
}

func ListAudit(c *gin.Context) {
	gc := ginlib.NewGinContext(c)
	tenantID := gc.Param("tenantID")
	if strings.Contains(gc.Request.Host, "admin") {
		aisToken := gc.GetAuthToken()
		selfUid := gc.GetUserID()
		err := quota.CheckIsAdminToken(gc, aisToken, selfUid)
		if err != nil {
			log.WithError(err).Error("check admin token failed")
			ctx.Error(c, err)
			return
		}
	}
	total, taas, err := algoresourceCtrl.ListAudit(gc, tenantID, gc.GetPagination(), gc.GetSort())
	if err != nil {
		log.WithError(err).Error("failed to ListAudit")
		ctx.Error(c, errors.ErrInternal)
		return
	}

	ctx.Success(c, listAuditResponse{
		Total: total,
		Items: taas,
	})
}

func randomSixString() string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 8)
	for i := range b {
		b[i] = letters[rand.Int63()%int64(8)]
	}
	return string(b)
}

type listDistributedAlgoResourceResponse struct {
	Items []*algoresourceType.TenantProjectAlgo `json:"items"`
}

type listDistributedTenantResourceResponse struct {
	Items []*algoresourceType.Content `json:"items"`
}

func ListDistributedAlgoResourceByID(c *gin.Context) {
	gc := ginlib.NewGinContext(c)
	algoType := gc.Param("algoType")
	recordID := gc.Param("recordID")

	metas, err := algoresourceCtrl.ListDistributedAlgoResourceByID(gc, algoType, recordID)
	if err != nil {
		log.WithError(err).Error("failed to ListAudit")
		ctx.Error(c, errors.ErrInternal)
		return
	}
	fmt.Println("metas::", metas)
	ctx.Success(c, listDistributedAlgoResourceResponse{
		Items: metas,
	})
}

func DeleteDistributedRecord(c *gin.Context) {
	gc := ginlib.NewGinContext(c)
	tenantID := gc.Param("tenantID")
	projectID := gc.Param("projectID")
	algoType := gc.Param("algoType")
	recordID := gc.Param("recordID")

	if err := algoresourceCtrl.DeleteDistributedRecord(gc, tenantID, projectID, algoType, recordID); err != nil {
		log.WithError(err).Error("failed to DeleteDistributedRecord")
		ctx.Error(c, errors.ErrInternal)
		return
	}

	ctx.Success(c, nil)
}

func GetDistributedTenantRecord(c *gin.Context) {
	gc := ginlib.NewGinContext(c)

	tenantID := gc.Param("tenantID")
	algoType := gc.Param("algoType")
	recordID := gc.Param("recordID")

	metas, err := algoresourceCtrl.GetDistributedTenantRecord(gc, tenantID, algoType, recordID)
	if err != nil {
		log.WithError(err).Error("failed to GetDistributedTenantRecord")
		ctx.Error(c, errors.ErrInternal)
		return
	}
	ctx.Success(c, listDistributedTenantResourceResponse{
		Items: metas,
	})
}

func DeleteDistributedTenantRecord(c *gin.Context) {
	gc := ginlib.NewGinContext(c)

	tenantID := gc.Param("tenantID")
	algoType := gc.Param("algoType")
	recordID := gc.Param("recordID")

	if err := algoresourceCtrl.DeleteDistributedTenantRecord(gc, tenantID, algoType, recordID); err != nil {
		log.WithError(err).Error("failed to DeleteDistributedRecord")
		ctx.Error(c, errors.ErrInternal)
		return
	}

	ctx.Success(c, nil)
}
