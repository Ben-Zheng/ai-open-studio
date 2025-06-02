package resource

import (
	"context"
	"encoding/json"
	errors2 "errors"
	"fmt"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/controller/quota"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sTypes "k8s.io/apimachinery/pkg/types"

	authv1 "go.megvii-inc.com/brain/brainpp/projects/aiservice/auth/pkg/api/v1"
	authTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/auth/pkg/app/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/features"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/oss"
	aisConst "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
	aisUtils "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/utils"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/consts"
	resourceCtrl "go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/controller/resource"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types/ctx"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types/ctx/errors"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types/resource"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/utils"
)

func Detail(c *gin.Context) {
	gc := ginlib.NewGinContext(c)
	var res struct {
		Used             *resource.Used `bson:"used" json:"used"`
		Total            *resource.Used `bson:"total" json:"total"`
		Accumulate       *resource.Used `bson:"accumulate" json:"accumulate"`
		RecentAccumulate *resource.Used `bson:"recentAccumulate" json:"recentAccumulate"`
	}

	userID := c.Query("userID")
	tenantID := c.Query("tenantID")

	if gc.GetAuthTenantID() != "" {
		tenantID = gc.GetAuthTenantID()
	}

	groupBy := "all"
	if userID != "" {
		groupBy = consts.CategoryUser
	} else if tenantID != "" {
		groupBy = consts.CategoryTenant
	}

	_, rs, err := resourceCtrl.ListResource(gc, 1, 1, groupBy, "_id", "", "", userID, tenantID)
	if err != nil {
		log.WithError(err).Error("failed to ListResource")
		ctx.Error(c, err)
		return
	}
	if len(rs) > 0 {
		res.Used = &rs[0].UsedQuota
	}

	bil, err := resourceCtrl.GetBillByGroup(gc, groupBy, userID, tenantID)
	if err != nil {
		log.WithError(err).Error("failed to GetBillByGroup")
		ctx.Error(c, err)
		return
	}
	res.Accumulate = &resource.Used{
		CPU:     bil.Accumulate.CPU + bil.AccumulateAdvance.CPU*consts.AccumulateAdvanceCPU,
		GPU:     bil.Accumulate.GPU + bil.AccumulateAdvance.GPU*consts.AccumulateAdvanceGPU,
		VirtGPU: bil.Accumulate.VirtGPU + bil.AccumulateAdvance.VirtGPU*consts.AccumulateAdvanceGPU,
		Memory:  bil.Accumulate.Memory + bil.AccumulateAdvance.Memory*consts.AccumulateAdvanceMemory,
		SSD:     bil.Accumulate.SSD + bil.AccumulateAdvance.SSD*consts.AccumulateAdvanceSSD,
		HDD:     bil.Accumulate.HDD + bil.AccumulateAdvance.CPU*consts.AccumulateAdvanceHDD,
	}

	ch, err := resourceCtrl.GetRecentAccumulate(gc, groupBy, userID, tenantID)
	if err != nil {
		log.WithError(err).Error("failed to GetRecentAccumulate")
		ctx.Error(c, err)
		return
	}
	res.RecentAccumulate = &ch.UsedQuota

	total, err := resourceCtrl.GetNodeTotal(gc)
	if err != nil {
		log.WithError(err).Error("failed to GetNodeTotal")
		ctx.Error(c, err)
		return
	}
	total.HDD += total.SSD
	if total.HDD == 0 || total.HDD < res.Used.HDD {
		total.HDD = -1
	}
	if total.SSD == 0 || total.SSD < res.Used.SSD {
		total.SSD = -1
	}

	_, nds, err := resourceCtrl.ListNode(gc, "", "", false, true, true, nil)
	if err != nil {
		log.WithError(err).Error("failed to ListNode")
		ctx.Error(c, err)
		return
	}
	for i := range nds {
		total.CPU -= nds[i].CPU.Usable
		total.GPU -= nds[i].GPU.Usable
		total.VirtGPU -= nds[i].VirtGPU.Usable
		total.Memory -= nds[i].Memory.Usable
		// 这里的计算逻辑应该不太对...
		for _, ssd := range nds[i].SSDs {
			total.HDD -= ssd.Usable
			total.SSD -= ssd.Usable
		}
		for _, hdd := range nds[i].HDDs {
			total.HDD -= hdd.Usable
			total.SSD -= hdd.Usable
		}
	}

	res.Total = total
	ctx.Success(c, res)
}

func ListResource(c *gin.Context) {
	gc := ginlib.NewGinContext(c)
	var listRes struct {
		Total int64               `json:"total"`
		Items []resource.Resource `json:"items"`
	}
	page, err := strconv.Atoi(c.Query("page"))
	if err != nil || page <= 0 {
		page = 1
	}
	pageSize, err := strconv.Atoi(c.Query("pageSize"))
	if err != nil || pageSize <= 0 {
		pageSize = 10
	}
	sortBy := c.Query("sortBy")
	if sortBy != string(types.KubeResourceCPU) && sortBy != types.ResourceMemory && sortBy != types.ResourceGPU && sortBy != types.ResourceHDD && sortBy != types.ResourceSSD {
		sortBy = ""
	}
	groupBy := c.Query("groupBy")
	if groupBy != consts.CategoryUser && groupBy != consts.CategoryTenant {
		listRes.Total, listRes.Items, err = resourceCtrl.ListAllResource(gc, page, pageSize, true)
		if err != nil {
			log.WithError(err).Error("failed to ListResource")
			ctx.Error(c, err)
			return
		}

		ctx.Success(c, listRes)
		return
	}

	listRes.Total, listRes.Items, err = resourceCtrl.ListResource(gc, page, pageSize, groupBy, sortBy, c.Query("userName"), c.Query("tenantName"), "", "")
	if err != nil {
		log.WithError(err).Error("failed to ListResource")
		ctx.Error(c, err)
		return
	}
	for i := range listRes.Items {
		if listRes.Items[i].TenantID == consts.NoneID {
			listRes.Items[i].MemberCount = 0
		}
	}

	ctx.Success(c, listRes)
}

func ListAudit(c *gin.Context) {
	gc := ginlib.NewGinContext(c)
	var listRes struct {
		Total int64            `json:"total"`
		Items []resource.Audit `json:"items"`
	}

	userIDs := c.QueryArray("userID")
	tenantIDs := c.QueryArray("tenantID")

	// 用户端
	tenantID := gc.GetAuthTenantID()
	if tenantID != "" {
		tenantIDs = []string{tenantID}

		// 越权检查
		selfUid := gc.GetUserID()
		if selfUid == "" {
			log.Error("get users failed")
			ctx.Error(c, errors2.New("get users failed"))
			return
		}
		err := quota.CheckAuth(gc, tenantID, selfUid)
		if err != nil {
			log.WithError(err).Error("failed to CheckAuth")
			ctx.Error(c, err)
			return
		}
	}

	resources := c.QueryArray("resource")
	operations := c.QueryArray("operation")

	if userName := c.Query("userName"); userName != "" && len(userIDs) == 0 {
		var err error
		userIDs, err = getUserIDsByUserName(gc, userName)
		if err != nil {
			log.WithError(err).Error("failed to getUserIDByUserName")
			ctx.Error(c, err)
			return
		}
		if len(userIDs) == 0 {
			ctx.Success(c, listRes)
			return
		}
	}

	datasetIDs := []string{}
	if datasetName := c.Query("datasetName"); datasetName != "" {
		var err error
		datasetIDs, err = getDatasetIDsByName(gc, datasetName, tenantIDs)
		if err != nil {
			log.WithError(err).Error("failed to getDatasetIDsByName")
			ctx.Error(c, err)
			return
		}
		if len(datasetIDs) == 0 {
			ctx.Success(c, listRes)
			return
		}
	}

	datasetRvIDs := c.QueryArray("datasetRvID")

	var err error
	listRes.Total, listRes.Items, err = resourceCtrl.ListAudit(gc, userIDs, tenantIDs, resources, operations, datasetIDs, datasetRvIDs)
	if err != nil {
		log.WithError(err).Error("failed to ListAudit")
		ctx.Error(c, err)
		return
	}

	for i := range listRes.Items {
		us, err := resourceCtrl.GetUserByID(listRes.Items[i].UserID)
		if err != nil {
			log.WithError(err).Error("failed to getUserByID")
		} else {
			listRes.Items[i].UserName = us.Name
		}

		if val, ok := consts.ResourceMap[listRes.Items[i].Resource]; ok {
			listRes.Items[i].Resource = string(val)
		}

		listRes.Items[i].TenantName, _ = getTenantNameByTenantID(gc, listRes.Items[i].TenantID)

		for _, ds := range listRes.Items[i].Datasets {
			dsNameB, _ := consts.DatasetNameCache.Get([]byte(ds.ID))
			rvNameB, _ := consts.DatasetNameCache.Get([]byte(ds.RvID))
			dsName, rvName := string(dsNameB), string(rvNameB)
			if dsName == "" || rvName == "" {
				// call datahub-apiserver to get dataset and revision Name
				dsName, rvName, _ = getDatasetNameByID(gc, ds.ID, ds.RvID, listRes.Items[i].TenantID, listRes.Items[i].ProjectID, ds.Level)
				if dsName != "" {
					consts.DatasetNameCache.Set([]byte(ds.ID), []byte(dsName), consts.DatasetNameCacheTTL)
				}
				if rvName != "" {
					consts.DatasetNameCache.Set([]byte(ds.RvID), []byte(rvName), consts.DatasetNameCacheTTL)
				}
			}
			ds.Name = fmt.Sprintf("%s %s", dsName, rvName)
		}

		if aisUtils.IsPhoneNum(listRes.Items[i].UserName) {
			listRes.Items[i].UserName = aisUtils.EncryptPhone(listRes.Items[i].UserName)
		}
	}

	ctx.Success(c, listRes)
}

func getUserIDsByUserName(gc *ginlib.GinContext, userName string) ([]string, error) {
	page := 1
	pageSize := 1000
	var userIDs []string
	for {
		_, us, err := authv1.NewClientWithAuthorazation(consts.ConfigMap.AuthServer,
			consts.ConfigMap.MultiSiteConfig.CenterAK, consts.ConfigMap.MultiSiteConfig.CenterSK).ListUser(gc, "createdAt", "desc", userName, "", page, pageSize)
		if err != nil {
			log.WithError(err).Error("failed to ListUser")
			return nil, err
		}

		for _, u := range us {
			userIDs = append(userIDs, u.ID)
		}

		if len(us) < pageSize {
			break
		}
	}

	return userIDs, nil
}

func getTenantNameByTenantID(gc *ginlib.GinContext, tenantID string) (string, error) {
	tenant, err := authv1.NewClientWithAuthorazation(consts.ConfigMap.AuthServer,
		consts.ConfigMap.MultiSiteConfig.CenterAK, consts.ConfigMap.MultiSiteConfig.CenterSK).GetTenant(gc, tenantID)
	if err != nil {
		log.WithError(err).Error("failed to GetTenant")
		return "", err
	}

	return tenant.Name, nil
}

func getDatasetNameByID(gc *ginlib.GinContext, datasetID, datasetRvID, tenant, project, level string) (string, string, error) {
	gcClone := ginlib.Copy(gc)
	gcClone.Set(authTypes.AISTenantHeader, tenant)
	gcClone.Set(authTypes.AISProjectHeader, project)
	meta, _, err := consts.DatahubClient.GetDataset(gcClone, datasetID, datasetRvID, level)
	if err != nil {
		log.WithFields(log.Fields{
			"datasetID":   datasetID,
			"datasetRvID": datasetRvID,
			"tenant":      tenant,
			"project":     project,
			"sourceLevel": level,
		}).WithError(err).Error("failed to get dataset and revision name")
		return "", "", err
	}
	return meta.Name, meta.Revisions[0].RevisionName, nil
}

func getDatasetIDsByName(gc *ginlib.GinContext, datasetName string, tenantIDs []string) (datasetIDs []string, err error) {
	// 如果未指定团队，则聚合所有 sourceLevel 为 Project 且使用数据集的审计事件的 tenantID，查询对应数据集名字
	tmpTenants, _ := resourceCtrl.GetTenantIDsWithDatasetsNotEmtpy(gc)
	tenants := []*resourceCtrl.TenantProject{}
	if len(tenantIDs) != 0 {
		for _, t := range tenantIDs {
			for _, tmpT := range tmpTenants {
				if tmpT.TenantID == t {
					tenants = append(tenants, tmpT)
				}
			}
		}
	} else {
		tenants = tmpTenants
	}

	for _, tenant := range tenants {
		gcClone := ginlib.Copy(gc)
		gcClone.Set(authTypes.AISTenantHeader, tenant.TenantID)
		gcClone.Set(authTypes.AISProjectHeader, tenant.ProjectID)
		log.WithField("tenantID", tenant.TenantID).WithField("projectID", tenant.ProjectID).Info("search dataset id by name")
		projectDatasets, _, err := consts.DatahubClient.QueryDatasetSimples(gcClone, []string{datasetName}, aisConst.ProjectLevel)
		if err != nil {
			log.WithError(err).Error("failed to get dataset id by name")
			return nil, err
		}
		for _, ds := range projectDatasets {
			datasetIDs = append(datasetIDs, ds.ID.Hex())
		}
	}
	// 查询平台数据集名字
	gcClone := ginlib.Copy(gc)
	systemDatasets, _, err := consts.DatahubClient.QueryDatasetSimples(gcClone, []string{datasetName}, aisConst.SystemLevel)
	if err != nil {
		return nil, err
	}
	for _, ds := range systemDatasets {
		datasetIDs = append(datasetIDs, ds.ID.Hex())
	}

	return datasetIDs, nil
}

func CreateAudit(c *gin.Context) {
	gc := ginlib.NewGinContext(c)

	var au resource.Audit
	if err := gc.Bind(&au); err != nil {
		log.WithError(err).Error("failed to ShouldBind")
		ctx.Error(c, err)
		return
	}
	if au.TenantID == "" || au.UserID == "" || au.ProjectID == "" ||
		au.Resource == "" || au.ResourceName == "" || au.Operation == "" || au.Timestamp == 0 {
		log.WithField("tenantID", au.TenantID).WithField("userID", au.UserID).WithField("projectID", au.ProjectID).
			WithField("rescourceType", au.Resource).WithField("resourceName", au.ResourceName).
			WithField("operation", au.Operation).WithField("timestamp", au.Timestamp).Error("param invalid")
		ctx.Error(c, errors.ErrParamsInvalid)
		return
	}

	if err := resourceCtrl.CreateAudit(gc, &au); err != nil {
		log.WithError(err).Error("failed to CreateAudit")
		ctx.Error(c, err)
		return
	}

	ctx.Success(c, nil)
}

func dealRowMetrics(c *gin.Context) (*resource.PromResponse, error) {
	gc := ginlib.NewGinContext(c)
	namespace := features.GetProjectWorkloadNamespace(gc.GetAuthProjectID())
	start, err := strconv.ParseFloat(c.Query("start"), 64)
	if err != nil {
		log.WithError(err).Error("failed to ParseInt")
		return nil, err
	}

	end, err := strconv.ParseFloat(c.Query("end"), 64)
	if err != nil {
		log.WithError(err).Error("failed to ParseInt")
		return nil, err
	}

	step, err := strconv.ParseFloat(c.Query("step"), 64)
	if err != nil {
		log.WithError(err).Error("failed to ParseInt")
		return nil, err
	}

	hasNamespace, err := strconv.ParseBool(c.Query("hasNamespace"))
	if err != nil {
		hasNamespace = false
	}

	query := c.Query("query")
	if hasNamespace {
		query = strings.Replace(query, "PROJECT_NAMESPACE", namespace, 1)
	}

	prome, err := resource.GetPromResponse(consts.ConfigMap.PrometheusServer, query, int64(start), int64(end), int64(step))
	if err != nil {
		log.Error("failed to GetPromResponse")
		return nil, err
	}
	return prome, nil
}

// GetMetrics 支持场景:
// 1 运营端：机器管理->业务资源监控
// 2 运营端：资源管理-><团队name>->团队资源->占用统计
// 3 用户端: 团队详情->团队资源->占用统计
func GetMetrics(c *gin.Context) {
	gc := ginlib.NewGinContext(c)
	if c.Query("query") != "" {
		prome, err := dealRowMetrics(c)
		if err != nil {
			log.WithError(err).Error("failed to dealRowMetrics")
			ctx.Error(c, err)
			return
		}
		c.JSON(http.StatusOK, prome)
		return
	}

	start, err := strconv.ParseInt(c.Query("start"), 10, 64)
	if err != nil {
		start = time.Now().Add(-time.Hour).Unix() * 1000
	}
	start /= 1000

	end, err := strconv.ParseInt(c.Query("end"), 10, 64)
	if err != nil {
		end = time.Now().Unix() * 1000
	}
	end /= 1000

	step, err := strconv.ParseInt(c.Query("step"), 10, 64)
	if err != nil {
		step = 30
	}
	step /= 1000

	categoryType := c.Query("category")
	resourceType := c.Query("resource")
	if consts.QueryProme[categoryType] == nil || consts.QueryProme[categoryType][resourceType] == nil {
		log.Error("category param invalid")
		ctx.Error(c, errors.ErrParamsInvalid)
		return
	}

	queryFields := consts.QueryProme[categoryType][resourceType].Params
	queryParam := make([]any, 0)

	for i := range queryFields {
		if queryFields[i] == "tenantID" && ((categoryType == consts.CategoryTenant || categoryType == consts.CategoryTenantQuota) && gc.GetAuthTenantID() != "") {
			// 用户端的团队资源占用监控 或者配额占用监控
			queryParam = append(queryParam, gc.GetAuthTenantID())
			// 越权校验
			selfUid := gc.GetUserID()
			if selfUid == "" {
				log.Error("get users failed")
				ctx.Error(c, errors2.New("get users failed"))
				return
			}
			err = quota.CheckAuth(gc, gc.GetAuthTenantID(), selfUid)
			if err != nil {
				log.WithError(err).Error("failed to CheckAuth")
				return
			}
		} else {
			queryParam = append(queryParam, c.Query(queryFields[i]))
		}
	}

	query := fmt.Sprintf(consts.QueryProme[categoryType][resourceType].QL, queryParam...)
	prome, err := resource.GetPromResponse(consts.ConfigMap.PrometheusServer, query, start, end, step)
	if err != nil {
		log.Error("category param invalid")
		ctx.Error(c, err)
		return
	}
	if len(prome.Data.Result) == 0 {
		log.Error("result not exsit")
		ctx.Success(c, resource.Data{})
		return
	}

	var data resource.Data
	var result resource.Result

	isBytes := func(rt string) bool {
		return rt == types.ResourceMemory || rt == types.ResourceSSD || rt == types.ResourceHDD ||
			rt == types.ResourceStorage || rt == types.ResourceDatasetStorage
	}

	for _, resultValue := range prome.Data.Result[0].Values {
		if len(resultValue) != 2 {
			continue
		}

		var timestampMicro int64
		if val, ok := resultValue[0].(float64); ok {
			timestampMicro = int64(1000 * val)
		}

		var value string
		if sval, ok := resultValue[1].(string); ok {
			val, err := strconv.ParseFloat(sval, 64)
			if err != nil {
				log.WithError(err).Error("failed to ParseFloat")
				continue
			}

			if categoryType == consts.CategoryNode {
				val *= 100
			} else if isBytes(resourceType) {
				val /= 1024 * 1024 * 1024
			}

			value = fmt.Sprintf("%.3f", val)
		}

		result.Values = append(result.Values, []any{timestampMicro, value})
	}
	data.Result = append(data.Result, result, prome.Data.Result[0])

	ctx.Success(c, data)
}

func GetPodMetrics(c *gin.Context) {
	start, end, step := time.Now().Add(-time.Hour).Unix(), time.Now().Unix(), int64(30)

	if startFloat, err := strconv.ParseFloat(c.Query("start"), 64); err == nil {
		start = int64(startFloat)
	}

	if endFloat, err := strconv.ParseFloat(c.Query("end"), 64); err == nil {
		end = int64(endFloat)
	}

	if stepFloat, err := strconv.ParseFloat(c.Query("step"), 64); err == nil {
		step = int64(stepFloat)
	}

	categoryType := consts.CategoryPod
	resourceType := c.Query("resource")
	if consts.QueryProme[categoryType][resourceType] == nil {
		log.Error("category param invalid")
		ctx.Error(c, errors.ErrParamsInvalid)
		return
	}
	podName := c.Param("podName")

	query := fmt.Sprintf(consts.QueryProme[categoryType][resourceType].QL, podName)
	prome, err := resource.GetPromResponse(consts.ConfigMap.PrometheusServer, query, start, end, step)
	if err != nil {
		log.Error("category param invalid")
		ctx.Error(c, err)
		return
	}
	if len(prome.Data.Result) == 0 {
		log.Error("result not exsit")
		ctx.Success(c, resource.Data{})
		return
	}

	var data resource.Data
	var result resource.Result
	for _, resultValue := range prome.Data.Result[0].Values {
		if len(resultValue) != 2 {
			continue
		}

		var timestamp int64
		if val, ok := resultValue[0].(float64); ok {
			timestamp = int64(val)
		}

		var value string
		if sval, ok := resultValue[1].(string); ok {
			val, err := strconv.ParseFloat(sval, 64)
			if err != nil {
				log.WithError(err).Error("failed to ParseFloat")
				continue
			}

			value = fmt.Sprintf("%.3f", val)
		}

		result.Values = append(result.Values, []any{timestamp, value})
	}
	data.Result = append(data.Result, result, prome.Data.Result[0])

	ctx.Success(c, data)
}

func ListNodes(c *gin.Context) {
	gc := ginlib.NewGinContext(c)
	var listRes struct {
		Total int64           `json:"total"`
		Items []resource.Node `json:"items"`
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

	var err error
	listRes.Total, listRes.Items, err = resourceCtrl.ListNode(gc, c.Query("nodeID"), c.Query("nodeName"), true, true, false, gc.GetPagination())
	if err != nil {
		log.WithError(err).Error("failed to ListNode")
		ctx.Error(c, err)
		return
	}

	ctx.Success(c, listRes)
}

func ManageNode(c *gin.Context) {
	var req struct {
		Unschedulable bool `json:"unschedulable"`
	}

	if err := c.BindJSON(&req); err != nil {
		log.WithError(err).Error("failed to BindQuery")
		ctx.Error(c, err)
		return
	}

	gc := ginlib.NewGinContext(c)
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

	_, ns, err := resourceCtrl.ListNode(gc, c.Param("nodeID"), "", false, true, false, &ginlib.Pagination{Skip: 0, Limit: 1})
	if err != nil {
		log.WithError(err).Error("failed to ListNode")
		ctx.Error(c, err)
		return
	}
	if len(ns) == 0 {
		log.Error("not exsit node")
		ctx.Error(c, errors.ErrParamsInvalid)
		return
	}
	if ns[0].UnSchedulable == req.Unschedulable {
		ctx.Success(c, nil)
		return
	}

	patchData := map[string]any{
		"spec": map[string]any{
			"unschedulable": req.Unschedulable,
		},
	}
	playLoadBytes, err := json.Marshal(patchData)
	if err != nil {
		log.WithError(err).Error("not exsit node")
		ctx.Error(c, errors.ErrParamsInvalid)
		return
	}

	coreV1Client, err := utils.NewCoreV1Client()
	if err != nil {
		log.WithError(err).Error("failed to NewCoreV1Client")
		ctx.Error(c, errors.ErrInternal)
		return
	}

	_, err = coreV1Client.Nodes().Patch(context.TODO(), ns[0].Name, k8sTypes.StrategicMergePatchType, playLoadBytes, metav1.PatchOptions{})
	if err != nil {
		log.WithError(err).Error("not exsit node")
		ctx.Error(c, errors.ErrParamsInvalid)
		return
	}

	if err := resourceCtrl.SetNodeUnschedulable(gc, ns[0].UUID, req.Unschedulable); err != nil {
		log.WithError(err).Error("failed to ListNode")
		ctx.Error(c, err)
		return
	}

	ctx.Success(c, nil)
}

func NodeDetail(c *gin.Context) {
	gc := ginlib.NewGinContext(c)
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

	_, ns, err := resourceCtrl.ListNode(gc, c.Param("nodeID"), "", false, true, false, &ginlib.Pagination{Skip: 0, Limit: 1})
	if err != nil {
		log.WithError(err).Error("failed to ListNode")
		ctx.Error(c, err)
		return
	}
	if len(ns) == 0 {
		log.Error("not exsit node")
		ctx.Error(c, errors.ErrParamsInvalid)
		return
	}

	ctx.Success(c, ns[0])
}

func ListNodePods(c *gin.Context) {
	gc := ginlib.NewGinContext(c)
	var listRes struct {
		Total int             `json:"total"`
		Items []*resource.Pod `json:"items"`
	}

	_, ns, err := resourceCtrl.ListNode(gc, c.Param("nodeID"), "", false, true, false, &ginlib.Pagination{Skip: 0, Limit: 1})
	if err != nil {
		log.WithError(err).Error("failed to ListNode")
		ctx.Error(c, err)
		return
	}
	if len(ns) == 0 {
		log.Error("not exsit node")
		ctx.Error(c, errors.ErrParamsInvalid)
		return
	}

	page, err := strconv.Atoi(c.Query("page"))
	if err != nil || page <= 0 {
		page = 1
	}
	pageSize, err := strconv.Atoi(c.Query("pageSize"))
	if err != nil || pageSize <= 0 {
		pageSize = 10
	}

	sortBy := c.Query("sortBy")
	if !types.PodSortMap[sortBy] {
		sortBy = ""
	}
	order := c.Query("order")
	if order != "asc" && order != "desc" {
		order = "asc"
	}
	owner := c.Query("owner")
	phases := c.QueryArray("phase")
	resourcetype := c.QueryArray("resourcetype")
	listRes.Total, listRes.Items, err = dealPods(ns[0].Pods, resourcetype, phases, owner, sortBy, order, page, pageSize)
	if err != nil {
		log.WithError(err).Error("failed to dealPods")
		ctx.Error(c, err)
		return
	}

	ctx.Success(c, listRes)
}

func filterByOwner(aps *resource.Pod, owner string) bool {
	if owner == "" || owner == "user" && aps.UserID != consts.NoneID ||
		owner == "system" && aps.UserID == consts.NoneID {
		return true
	}

	return false
}

func dealPods(aps []resource.Pod, resourcetype, phases []string, owner, sortBy, order string, page, pageSize int) (int, []*resource.Pod, error) {
	var ps []*resource.Pod
	rtm := make(map[string]bool)
	for _, rt := range resourcetype {
		rtm[rt] = true
	}
	pm := make(map[string]bool)
	for _, p := range phases {
		pm[p] = true
	}

	for i := range aps {
		if (len(resourcetype) == 0 || len(resourcetype) > 0 && rtm[aps[i].Resourcetype]) &&
			(len(phases) == 0 || len(phases) > 0 && pm[aps[i].Phase]) && filterByOwner(&aps[i], owner) {
			ps = append(ps, &aps[i])
		}
	}

	if sortBy != "" {
		sort.SliceStable(ps, func(i, j int) bool {
			var result bool

			switch sortBy {
			case types.ResourceCPU:
				result = ps[i].UsedQuota.CPU < ps[j].UsedQuota.CPU
			case types.ResourceAPU:
				result = ps[i].UsedQuota.APU < ps[j].UsedQuota.APU
			case types.ResourceNPU:
				result = ps[i].UsedQuota.NPU < ps[j].UsedQuota.NPU
			case types.ResourceGPU:
				result = ps[i].UsedQuota.GPU < ps[j].UsedQuota.GPU
			case types.ResourceVirtGPU:
				result = ps[i].UsedQuota.VirtGPU < ps[j].UsedQuota.VirtGPU
			case types.ResourceMemory:
				result = ps[i].UsedQuota.Memory < ps[j].UsedQuota.Memory
			}

			if order == "desc" {
				result = !result
			}

			return result
		})
	}

	start := (page - 1) * pageSize
	end := page * pageSize
	if start < 0 {
		start = 0
	}
	if end > len(ps) {
		end = len(ps)
	}
	if start >= end {
		return 0, []*resource.Pod{}, nil
	}

	return len(ps), ps[start:end], nil
}

func GetDetails(c *gin.Context) {
	searchTime, err := strconv.ParseInt(c.Query("searchTime"), 10, 64)
	if err != nil {
		log.WithError(err).Error("failed to ParseInt")
		ctx.Error(c, err)
		return
	}

	t := time.Unix(searchTime, 0)
	o := oss.OSS{
		Session:    consts.OSSSession,
		BucketName: resourceCtrl.NodeDetailBucketName,
		KeyName:    resourceCtrl.GetNodeDetailKey(&t),
	}

	data, err := o.DownloadForBytes()
	if err != nil {
		log.WithError(err).Error("failed to DownloadForBytes")
		ctx.Error(c, err)
		return
	}

	var re interface{}
	if err := json.Unmarshal(data, &re); err != nil {
		log.WithError(err).Error("failed to Unmarshal")
		ctx.Error(c, err)
		return
	}

	ctx.SuccessWithList(c, re)
}

func GetNodeDetails(c *gin.Context) {
	searchTime, err := strconv.ParseInt(c.Query("searchTime"), 10, 64)
	if err != nil {
		log.WithError(err).Error("failed to ParseInt")
		ctx.Error(c, err)
		return
	}

	t := time.Unix(searchTime, 0)
	o := oss.OSS{
		Session:    consts.OSSSession,
		BucketName: resourceCtrl.NodeDetailBucketName,
		KeyName:    resourceCtrl.GetPodDetailKey(&t, c.Param("nodeName")),
	}

	data, err := o.DownloadForBytes()
	if err != nil {
		log.WithError(err).Error("failed to DownloadForBytes")
		ctx.Error(c, err)
		return
	}

	var re interface{}
	if err := json.Unmarshal(data, &re); err != nil {
		log.WithError(err).Error("failed to Unmarshal")
		ctx.Error(c, err)
		return
	}

	ctx.SuccessWithList(c, re)
}
