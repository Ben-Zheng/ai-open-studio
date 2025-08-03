package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httputil"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"github.com/yhat/wsutil"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	corev1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"

	authv1 "go.megvii-inc.com/brain/brainpp/projects/aiservice/auth/pkg/api/v1"
	authTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/auth/pkg/app/types"
	codehubV1 "go.megvii-inc.com/brain/brainpp/projects/aiservice/codehub/pkg/api/v1"
	codehubTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/codehub/pkg/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ajob"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/auditlib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/features"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	aisConst "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
	publicTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/publicservice/pkg/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/workspace/pkg/apis/client"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/workspace/pkg/consts"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/workspace/pkg/types/ctx"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/workspace/pkg/types/ctx/errors"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/workspace/pkg/types/workspace"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/workspace/pkg/utils"
)

type ListWorkspaceResp struct {
	Total int64                  `json:"total"`
	Items []*workspace.Workspace `json:"items"`
}

func getWorkspaceSSHCMD(userID string, ws *workspace.Workspace) string {
	if userID == ws.UserID {
		return fmt.Sprintf("ssh -p %d %s@%s", consts.ConfigMap.WSSpec.SSHProxyPort, ws.ID.Hex(), consts.ConfigMap.Domain)
	}

	return fmt.Sprintf("ssh -p %d %s.%s@%s", consts.ConfigMap.WSSpec.SSHProxyPort, ws.ID.Hex(), userID, consts.ConfigMap.Domain)
}

// List 获取工作台列表
func List(c *gin.Context) {
	gc := ginlib.NewGinContext(c)
	name := c.GetString(consts.Name)
	userID := c.GetString(consts.UserID)
	tenantID := c.GetString(consts.TenantID)
	projectID := c.GetString(consts.ProjectID)
	onlyMe, _ := strconv.ParseBool(c.Query(consts.OnlyMe))

	us, err := authv1.NewClientWithAuthorazation(consts.ConfigMap.EndPoint.AuthServer,
		consts.ConfigMap.MultiSiteConfig.CenterAK, consts.ConfigMap.MultiSiteConfig.CenterSK).GetUser(gc, userID)
	if err != nil {
		log.WithError(err).Error("failed to GetUser")
		ctx.Error(c, errors.ErrParamsInvalid)
		return
	}

	var isAdmin bool
	for _, ten := range us.Tenants {
		if ten.ID != tenantID || ten.Role == nil {
			continue
		}

		if _, ok := authTypes.AdminRoleMap[authTypes.SystemRoleType(ten.Role.Name)]; ok {
			isAdmin = true
		}
	}

	total, wss, err := workspace.ListWorkspace(gc, isAdmin, onlyMe, userID, projectID, tenantID, name, c.GetInt(consts.Page), c.GetInt(consts.PageSize), c.GetString(consts.SortBy), c.GetString(consts.Order))
	if err != nil {
		log.WithError(err).Error("failed to ListWorkspace")
		ctx.Error(c, err)
		return
	}

	for _, ws := range wss {
		ws.SSHCMD = getWorkspaceSSHCMD(userID, ws)
		_, inss, err := workspace.ListInstance(gc, false, ws.ID, 1, 1)
		if err != nil {
			log.WithError(err).Error("failed to GetStartupInstance")
			ctx.Error(c, err)
			return
		}

		if len(inss) > 0 {
			ws.Instance = &inss[0]
			if ws.Instance.State == consts.InstanceStateRunning {
				ws.Instance.UsedDuration = time.Now().Unix() - ws.Instance.CreatedAt
			}

			if err := loadInstanceState(gc, ws.Instance); err != nil {
				log.WithError(err).Error("failed to loadInstanceState")
			}
		}
	}

	ctx.Success(c, &ListWorkspaceResp{
		Total: total,
		Items: wss,
	})
}

func checkoutResource(spec *workspace.Specification) bool {
	memory, cpu, gpu := resource.MustParse(fmt.Sprintf("%sGi", spec.MEM)), resource.MustParse(spec.CPU), resource.MustParse(spec.GPU)
	memory.Sub(resource.MustParse("232Mi"))
	memory.Sub(resource.MustParse(fmt.Sprintf("%dMi", cpu.Value()*8)))
	if gpu.Value() > 0 {
		memory.Sub(resource.MustParse("1024Mi"))
	}
	memory.Set(memory.Value()*512/513 - 1)
	return memory.Value() > 0
}

func Create(c *gin.Context) {
	gc := ginlib.NewGinContext(c)
	userID := c.GetString(consts.UserID)
	tenantID := c.GetString(consts.TenantID)
	projectID := c.GetString(consts.ProjectID)

	var req workspace.Workspace
	if err := gc.ShouldBindJSON(&req); err != nil {
		log.WithError(err).Error("failed to ShouldBindJSON")
		ctx.Error(c, err)
		return
	}

	if req.Level != workspace.PrivateLevel && req.Level != workspace.ProjectLevel {
		log.WithField("level", req.Level).Error("params is invalid")
		ctx.Error(c, errors.ErrParamsInvalid)
		return
	}

	// todo 还需要校验镜像
	if req.Name == "" {
		log.Error("create params invalid")
		ctx.Error(c, errors.ErrParamsInvalid)
		return
	}

	if _, err := workspace.GetWorkspaceByName(gc, userID, projectID, tenantID, req.Name); err == nil {
		log.WithError(err).Error("workspace has existed")
		ctx.Error(c, errors.ErrorWorkspaceExisted)
		return
	} else if err != mongo.ErrNoDocuments {
		log.WithError(err).Error("failed to GetWorkspaceByName")
		ctx.Error(c, errors.ErrInternal)
		return
	}

	cb, err := codehubV1.NewClient(consts.ConfigMap.EndPoint.CodeHub).GetCodebase(gc, req.CodeID, "", aisConst.SystemLevel)
	if err != nil {
		log.WithError(err).Error("failed to GetCodebase")
		ctx.Error(c, errors.ErrParamsInvalid)
		return
	}
	if cb.CodeType != aisConst.CodeTypeRootfs || len(cb.Revisions) != 1 || cb.Revisions[0].Ban ||
		consts.ConfigMap.WSSpec.Runtime != codehubTypes.GetWSRuntime(cb.Revisions[0].TaskClaims[aisConst.TaskTypeRootfs]) {
		log.WithError(err).Error("Codebase invalid")
		ctx.Error(c, errors.ErrParamsInvalid)
		return
	}

	if consts.ConfigMap.WSSpec.Runtime == aisConst.WSRuntimeKubevirt {
		if pass := checkoutResource(req.Spec); !pass {
			log.Error("resource invalid")
			ctx.Error(c, errors.ErrorWorkspaceMemory)
			return
		}
	}

	ws := &workspace.Workspace{
		ID:          primitive.NewObjectID(),
		UserID:      userID,
		TenantID:    tenantID,
		ProjectID:   projectID,
		Name:        req.Name,
		CodeID:      req.CodeID,
		Image:       cb.Revisions[0].ImageURL,
		Runtime:     consts.ConfigMap.WSSpec.Runtime,
		Description: req.Description,
		ExitTimeout: consts.ExitTimeoutH10,
		CreatedBy:   gc.GetUserName(),
		CreatedAt:   time.Now().Unix(),
		Spec: &workspace.Specification{
			CPU:      req.Spec.CPU,
			MEM:      fmt.Sprintf("%sGi", req.Spec.MEM),
			GPU:      req.Spec.GPU,
			Storage:  req.Spec.Storage,
			GPUTypes: req.Spec.GPUTypes,
		},
		Level: req.Level,
	}

	name := workspace.GetInstanceName(ws.ID)
	lbs, annotations := buildCommonLabelAndAnnotation(ws, projectID)
	namespace := features.GetProjectWorkloadNamespace(gc.GetAuthProjectID())
	if err := CreatePersistentVolumeClaim(name, namespace, lbs, annotations); err != nil {
		log.WithError(err).Error("failed to checkPersistentVolumeClaim")
		ctx.Error(c, err)
		return
	}

	if _, err := workspace.CreateWorkspace(gc, ws); err != nil {
		log.WithError(err).Error("failed to CreateWorkspace")
		ctx.Error(c, err)
		return
	}

	ctx.Success(c, nil)
	auditlib.GinContextSendAudit(c, aisConst.ResourceTypeWorkspace, req.Name, string(aisConst.OperationTypeCreate))
}

func Detail(c *gin.Context) {
	gc := ginlib.NewGinContext(c)
	userID := c.GetString(consts.UserID)
	tenantID := c.GetString(consts.TenantID)
	workspaceID, err := primitive.ObjectIDFromHex(c.GetString(consts.WorkspaceID))
	if err != nil {
		log.WithError(err).Error("failed to ObjectIDFromHex")
		ctx.Error(c, err)
		return
	}

	ws, err := workspace.GetWorkspaceByID(gc, workspaceID)
	if err != nil {
		log.WithError(err).Error("failed to GetInstanceByID")
		ctx.Error(c, err)
		return
	}
	if _, ok := consts.KeyInspectTypes[c.GetHeader(consts.KeyInspect)]; ok {
		tenantID = ws.TenantID
		if userID == "admin" {
			userID = ws.UserID
		}
	}
	if ws.TenantID != tenantID {
		log.Error("params is invalid")
		ctx.Error(c, errors.ErrParamsInvalid)
		return
	}

	if _, err := checkUserAuthority(gc, userID, ws); err != nil {
		log.WithError(err).Error("failed to checkUserAuthority")
		ctx.Error(c, err)
		return
	}

	if consts.KeyInspectTypes[c.GetHeader(consts.KeyInspect)] {
		keys, err := authv1.NewClientWithAuthorazation(consts.ConfigMap.EndPoint.AuthServer,
			consts.ConfigMap.MultiSiteConfig.CenterAK, consts.ConfigMap.MultiSiteConfig.CenterSK).ListSSHKey(gc, userID, c.GetHeader(consts.KeyType), c.GetHeader(consts.KeyMarshal))
		if err != nil {
			log.WithError(err).Error("failed to ListSSHKey")
			ctx.Error(c, err)
			return
		}
		if len(keys) == 0 {
			log.Error("no match ssh key")
			ctx.Error(c, errors.ErrParamsInvalid)
			return
		}
	}

	ws.SSHCMD = getWorkspaceSSHCMD(userID, ws)
	_, inss, err := workspace.ListInstance(gc, false, ws.ID, 1, 1)
	if err != nil {
		log.WithError(err).Error("failed to GetStartupInstance")
		ctx.Error(c, err)
		return
	}

	if len(inss) > 0 {
		ws.Instance = &inss[0]
		if ws.Instance.State == consts.InstanceStateRunning {
			ws.Instance.UsedDuration = time.Now().Unix() - ws.Instance.CreatedAt
		}

		if err := loadInstanceState(gc, ws.Instance); err != nil {
			log.WithError(err).Error("failed to loadInstanceState")
		}
	}

	ctx.Success(c, ws)
}

func Update(c *gin.Context) {
	gc := ginlib.NewGinContext(c)
	userID := c.GetString(consts.UserID)
	tenantID := c.GetString(consts.TenantID)
	projectID := c.GetString(consts.ProjectID)
	workspaceID, err := primitive.ObjectIDFromHex(c.GetString(consts.WorkspaceID))
	if err != nil {
		log.WithError(err).Error("failed to ObjectIDFromHex")
		ctx.Error(c, err)
		return
	}

	ws, err := workspace.GetWorkspaceByID(gc, workspaceID)
	if err != nil {
		log.WithError(err).Error("failed to GetInstanceByID")
		ctx.Error(c, err)
		return
	}
	if ws.TenantID != tenantID {
		log.Error("params is invalid")
		ctx.Error(c, errors.ErrParamsInvalid)
		return
	}

	if _, err := checkUserAuthority(gc, userID, ws); err != nil {
		log.WithError(err).Error("failed to checkUserAuthority")
		ctx.Error(c, err)
		return
	}

	var req workspace.Workspace
	if err := gc.ShouldBindJSON(&req); err != nil {
		log.WithError(err).Error("failed to ShouldBindJSON")
		ctx.Error(c, err)
		return
	}
	if req.Name == "" {
		log.Error("create params invalid")
		ctx.Error(c, errors.ErrParamsInvalid)
		return
	}

	if _, err := workspace.GetWorkspaceByName(gc, userID, projectID, tenantID, req.Name); err == nil {
		log.WithError(err).Error("workspace has existed")
		ctx.Error(c, errors.ErrorWorkspaceExisted)
		return
	} else if err != mongo.ErrNoDocuments {
		log.WithError(err).Error("failed to GetWorkspaceByName")
		ctx.Error(c, errors.ErrInternal)
		return
	}

	if err := workspace.UpdateWorkspaceName(gc, workspaceID, req.Name); err != nil {
		log.WithError(err).Error("failed to UpdateWorkspaceName")
		ctx.Error(c, err)
		return
	}

	ctx.Success(c, nil)
}

func Delete(c *gin.Context) {
	gc := ginlib.NewGinContext(c)
	userID := c.GetString(consts.UserID)
	tenantID := c.GetString(consts.TenantID)
	workspaceID, err := primitive.ObjectIDFromHex(c.GetString(consts.WorkspaceID))
	if err != nil {
		log.WithError(err).Error("failed to ObjectIDFromHex")
		ctx.Error(c, err)
		return
	}

	ws, err := workspace.GetWorkspaceByID(gc, workspaceID)
	if err != nil {
		log.WithError(err).Error("failed to GetInstanceByID")
		ctx.Error(c, err)
		return
	}
	if ws.TenantID != tenantID {
		log.Error("params is invalid")
		ctx.Error(c, errors.ErrParamsInvalid)
		return
	}

	if _, err := checkUserAuthority(gc, userID, ws); err != nil {
		log.WithError(err).Error("failed to checkUserAuthority")
		ctx.Error(c, err)
		return
	}

	_, inss, err := workspace.ListInstance(gc, true, ws.ID, 1, 1)
	if err != nil {
		log.WithError(err).Error("failed to GetStartupInstance")
		ctx.Error(c, err)
		return
	}

	if len(inss) != 0 {
		log.Error("exist instance starting up")
		ctx.Error(c, errors.ErrorWorkspaceStartingUp)
		return
	}

	name := workspace.GetInstanceName(ws.ID)
	namespace := features.GetProjectWorkloadNamespace(gc.GetAuthProjectID())
	if err := DeletePersistentVolumeClaim(name, namespace); err != nil {
		log.WithError(err).Error("failed to DeletePersistentVolumeClaim")
		ctx.Error(c, err)
		return
	}

	if err := consts.Clientsets.WSClient.Workspaces().Delete(name, namespace); !k8sErrors.IsNotFound(err) && err != nil {
		log.WithError(err).Error("failed to Delete Workspaces")
		ctx.Error(c, err)
		return
	}

	if err := workspace.DeleteWorkspace(gc, workspaceID); err != nil {
		log.WithError(err).Error("failed to DeleteWorkspace")
		ctx.Error(c, err)
		return
	}

	ctx.Success(c, nil)
	auditlib.GinContextSendAudit(c, aisConst.ResourceTypeWorkspace, ws.Name, string(aisConst.OperationTypeDelete))
}

func ReleaseByUser(c *gin.Context) {
	gc := ginlib.NewGinContext(c)

	req := gc.GetReleaseByUserRequest()
	if req == nil {
		log.Error("failed to GetReleaseByUserRequest")
		ctx.Error(c, errors.ErrParamsInvalid)
		return
	}

	fake := ginlib.NewMockGinContext()
	fake.SetTenantName(req.TenantID)

	wss, err := workspace.ListWorkspaceByUser(fake, req.UserID)
	if err != nil {
		log.WithError(err).Error("failed to ListWorkspaceByUser")
		ctx.Error(c, err)
		return
	}

	log.Infof("release workspace found: %d", len(wss))

	for _, ws := range wss {
		log.Infof("release workspace: %+v", ws.ID.Hex())
		name := workspace.GetInstanceName(ws.ID)
		namespace := features.GetProjectWorkloadNamespace(ws.ProjectID)
		if err := DeletePersistentVolumeClaim(name, namespace); err != nil {
			log.WithError(err).Error("failed to DeletePersistentVolumeClaim")
			ctx.Error(c, err)
			return
		}
		log.Infof("release workspace: %+v, name: %+v", ws.ID.Hex(), name)
		if err := consts.Clientsets.WSClient.Workspaces().Delete(name, namespace); !k8sErrors.IsNotFound(err) && err != nil {
			log.WithError(err).Error("failed to Delete Workspaces")
			ctx.Error(c, err)
			return
		}

		if err := workspace.DeleteWorkspace(gc, ws.ID); err != nil {
			log.WithError(err).Error("failed to DeleteWorkspace")
			ctx.Error(c, err)
			return
		}
	}
	ctx.Success(c, nil)
}

type ListInstanceResp struct {
	Total int64                `json:"total"`
	Items []workspace.Instance `json:"items"`
}

// List 获取工作台启动实例列表
func ListInstance(c *gin.Context) {
	gc := ginlib.NewGinContext(c)
	userID := c.GetString(consts.UserID)
	tenantID := c.GetString(consts.TenantID)

	workspaceID, err := primitive.ObjectIDFromHex(c.GetString(consts.WorkspaceID))
	if err != nil {
		log.WithError(err).Error("failed to ObjectIDFromHex")
		ctx.Error(c, err)
		return
	}

	ws, err := workspace.GetWorkspaceByID(gc, workspaceID)
	if err != nil {
		log.WithError(err).Error("failed to GetWorkspaceByID")
		ctx.Error(c, err)
		return
	}
	if ws == nil || tenantID != ws.TenantID {
		log.Error("params is invalid")
		ctx.Error(c, errors.ErrParamsInvalid)
		return
	}
	if _, err := checkUserAuthority(gc, userID, ws); err != nil {
		log.WithError(err).Error("failed to checkUserAuthority")
		ctx.Error(c, err)
		return
	}

	total, inss, err := workspace.ListInstance(gc, false, workspaceID, c.GetInt(consts.Page), c.GetInt(consts.PageSize))
	if err != nil {
		log.WithError(err).Error("failed to ListInstance")
		ctx.Error(c, err)
		return
	}

	for i := range inss {
		if err := loadInstanceState(gc, &inss[i]); err != nil {
			log.WithError(err).Error("failed to loadInstanceState")
			ctx.Error(c, err)
			return
		}
	}

	ctx.Success(c, &ListInstanceResp{
		Total: total,
		Items: inss,
	})
}

func loadInstanceState(gc *ginlib.GinContext, ins *workspace.Instance) error {
	if ins.State == consts.InstanceStateCompleted || ins.State == consts.InstanceStateFailed {
		return nil
	}

	// 查看workspace状态，通过lastchargedAt来更新instance状态
	oldState := ins.State
	news, err := consts.Clientsets.WSClient.Workspaces().Get(ins.Name, ins.Namespace)
	if !k8sErrors.IsNotFound(err) && err != nil {
		log.WithError(err).Error("failed to Get Workspaces")
		return err
	}

	if news.Spec.WorkspaceID == "" {
		ins.State = consts.InstanceStateFailed
		ins.Reason = consts.ReasonNotFound
	} else if news.Name != ins.Name {
		ins.State = consts.InstanceStateFailed
		ins.Reason = consts.ReasonUnknown
	} else if news.Status.Status == string(consts.InstanceStateCompleted) ||
		news.Status.Status == string(consts.InstanceStatePending) && ins.CreatedAt+consts.InstancePendingDuration < time.Now().Unix() {
		ins.State = consts.InstanceStateCompleted
	} else if news.Status.Status == string(consts.InstanceStateRunning) && ins.CreatedAt+consts.InstanceDelayDuration < time.Now().Unix() {
		ins.State = consts.InstanceStateRunning
	} else {
		ins.State = consts.InstanceStatePending
	}

	if oldState != ins.State {
		log.Info("instance state changed")
		if err := workspace.SetInstanceState(gc, ins.ID, ins.State, ins.Reason); err != nil {
			log.WithError(err).Error("failed to SetInstanceState")
		}
	}

	if news.Spec.WorkspaceID != "" && news.Status.Status != string(consts.InstanceStateCompleted) &&
		(ins.State == consts.InstanceStateFailed || ins.State == consts.InstanceStateCompleted) {
		if err := consts.Clientsets.WSClient.Workspaces().Stop(ins.Name, ins.Namespace); err != nil {
			log.WithError(err).Error("failed to Stop Workspaces")
		}
	}

	return nil
}

// Startup 启动工作台
func Startup(c *gin.Context) {
	gc := ginlib.NewGinContext(c)
	userID := c.GetString(consts.UserID)
	tenantID := c.GetString(consts.TenantID)
	namespace := features.GetProjectWorkloadNamespace(gc.GetAuthProjectID())
	workspaceID, err := primitive.ObjectIDFromHex(c.GetString(consts.WorkspaceID))
	if err != nil {
		log.WithError(err).Error("failed to ObjectIDFromHex")
		ctx.Error(c, err)
		return
	}

	ws, err := workspace.GetWorkspaceByID(gc, workspaceID)
	if err != nil {
		log.WithError(err).Error("failed to GetWorkspaceByID")
		ctx.Error(c, err)
		return
	}

	if ws == nil || tenantID != ws.TenantID {
		log.Error("params is invalid")
		ctx.Error(c, errors.ErrParamsInvalid)
		return
	}

	if err := checkStartupState(gc, ws); err != nil {
		log.WithError(err).Error("failed to checkStartupState")
		ctx.Error(c, err)
		return
	}

	token := fmt.Sprintf("%s-%s", string(utils.RandStringBytesMaskImpr(8)), uuid.New().String())
	ins, err := workspace.CreateInstance(gc, ws.ID, namespace, token)
	if err != nil {
		log.WithError(err).Error("failed to CreateInstance")
		ctx.Error(c, err)
		return
	}

	cb, err := codehubV1.NewClient(consts.ConfigMap.EndPoint.CodeHub).GetCodebase(gc, ws.CodeID, "", aisConst.SystemLevel)
	if err != nil || cb.CodeType != aisConst.CodeTypeRootfs || len(cb.Revisions) != 1 || cb.Revisions[0].Ban ||
		consts.ConfigMap.WSSpec.Runtime != codehubTypes.GetWSRuntime(cb.Revisions[0].TaskClaims[aisConst.TaskTypeRootfs]) {
		log.WithError(err).WithField("cb", cb).Error("failed to GetCodebase")
		if err := workspace.SetInstanceState(gc, ins.ID, consts.InstanceStateFailed, consts.ReasonCodebaseInvalid); err != nil {
			log.WithError(err).Error("failed to SetInstanceState")
		}
		ctx.Error(c, errors.ErrParamsInvalid)
		return
	}

	us, err := checkUserAuthority(gc, userID, ws)
	if err != nil {
		log.WithError(err).Error("failed to GetCodebase")
		if err := workspace.SetInstanceState(gc, ins.ID, consts.InstanceStateFailed, consts.ReasonNoPermission); err != nil {
			log.WithError(err).Error("failed to SetInstanceState")
		}
		ctx.Error(c, errors.ErrParamsInvalid)
		return
	}

	if err := startup(ws, ins, cb, us, gc.GetAuthProjectID()); err != nil {
		log.WithError(err).Error("failed to startup")
		if err := workspace.SetInstanceState(gc, ins.ID, consts.InstanceStateFailed, consts.ReasonStartupFailed); err != nil {
			log.WithError(err).Error("failed to SetInstanceState")
		}
		ctx.Error(c, err)
		return
	}

	ctx.Success(c, nil)
	auditlib.GinContextSendAudit(c, aisConst.ResourceTypeWorkspace, ws.Name, string(aisConst.OperationTypeStart))
}

func checkUserAuthority(gc *ginlib.GinContext, userID string, ws *workspace.Workspace) (*authTypes.User, error) {
	if ws == nil {
		return nil, errors.ErrNotFound
	}

	us, err := authv1.NewClientWithAuthorazation(consts.ConfigMap.EndPoint.AuthServer,
		consts.ConfigMap.MultiSiteConfig.CenterAK, consts.ConfigMap.MultiSiteConfig.CenterSK).GetUser(gc, userID)
	if err != nil {
		log.WithError(err).Error("failed to GetUser")
		return nil, err
	}

	if us.Ban.Ban {
		return nil, errors.ErrNotFound
	}

	for _, tenant := range us.Tenants {
		if tenant.ID != ws.TenantID {
			continue
		}

		if userID == ws.UserID {
			if _, ok := authTypes.RoleMap[authTypes.SystemRoleType(tenant.Role.Name)]; ok {
				return us, nil
			}
		} else {
			if ws.Level == workspace.ProjectLevel {
				return us, nil
			}

			if _, ok := authTypes.AdminRoleMap[authTypes.SystemRoleType(tenant.Role.Name)]; ok {
				return us, nil
			}
		}
	}

	return nil, errors.ErrNotFound
}

func checkStartupState(gc *ginlib.GinContext, ws *workspace.Workspace) error {
	_, inss, err := workspace.ListInstance(gc, true, ws.ID, 1, 1)
	if err != nil {
		log.WithError(err).Error("failed to GetStartupInstance")
		return err
	}

	if len(inss) != 0 {
		if err := loadInstanceState(gc, &inss[0]); err != nil {
			log.WithError(err).Error("failed to loadInstanceState")
			return err
		}

		if inss[0].State == consts.InstanceStateRunning || inss[0].State == consts.InstanceStatePending {
			log.Error("exist startup instance")
			return errors.ErrorWorkspaceStartingUp
		}
	}

	return nil
}

func CreatePersistentVolumeClaim(name, namespace string, lbs, annotations map[string]string) error {
	pvcTemp := corev1.PersistentVolumeClaim{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Labels:      lbs,
			Annotations: annotations,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse(consts.ConfigMap.WSSpec.Capacity)},
				Limits:   corev1.ResourceList{corev1.ResourceStorage: resource.MustParse(consts.ConfigMap.WSSpec.Capacity)},
			},
			StorageClassName: &consts.ConfigMap.WSSpec.StorageClassName,
		},
	}

	if _, err := consts.Clientsets.K8sClient.CoreV1().PersistentVolumeClaims(namespace).Get(&gin.Context{}, name, metav1.GetOptions{}); err == nil {
		return nil
	} else if !k8sErrors.IsNotFound(err) {
		return err
	}

	_, err := consts.Clientsets.K8sClient.CoreV1().PersistentVolumeClaims(namespace).Create(context.Background(), &pvcTemp, metav1.CreateOptions{})

	return err
}

func DeletePersistentVolumeClaim(name, namespace string) error {
	if err := consts.Clientsets.K8sClient.CoreV1().PersistentVolumeClaims(namespace).Delete(context.Background(), name, metav1.DeleteOptions{}); err != nil && !k8sErrors.IsNotFound(err) {
		return err
	}

	return nil
}

func buildCommonLabelAndAnnotation(ws *workspace.Workspace, projectID string) (map[string]string, map[string]string) {
	lbs := map[string]string{
		"workspace":              ws.ID.Hex(),
		aisConst.AISProject:      projectID,
		aisConst.AISResourceType: string(aisConst.AIServiceTypeWorkspace),
		aisConst.AISUserID:       ws.UserID,
		aisConst.AISTenantID:     ws.TenantID,
		publicTypes.AISAdmissionObjectSelectorLabelKey: publicTypes.AISAdmissionObjectSelectorLabelValue,
	}

	annotations := map[string]string{}
	commonLabels := &aisConst.CommonLabels{
		UserID:       ws.CreatedBy,
		UserName:     ws.CreatedBy,
		TenantID:     ws.TenantID,
		TenantName:   ws.TenantID,
		ProjectID:    ws.ProjectID,
		ProjectName:  ws.ProjectID,
		ResourceType: aisConst.ResourceTypeWorkspace,
		ResourceID:   ws.ID.Hex(),
		ResourceName: ws.Name,
	}
	commonLabels.Inject(annotations, lbs)

	return lbs, annotations
}

func startup(ws *workspace.Workspace, ins *workspace.Instance, cb *codehubTypes.Codebase, us *authTypes.User, projectID string) error {
	mews, err := client.NewAISWorkspace()
	if err != nil {
		log.WithError(err).Error("failed to NewAISWorkspace")
		return err
	}

	lbs, annotations := buildCommonLabelAndAnnotation(ws, projectID)
	lbs[aisConst.AISWSApp] = ins.Name
	if consts.ConfigMap.WSSpec.Runtime == aisConst.WSRuntimeKubevirt {
		lbs[aisConst.AISWSInjectForbidden] = "yes"
	}
	annotations[aisConst.AISWSInstanceID] = ins.ID.Hex()
	annotations[aisConst.AISWSInstanceToken] = ins.Token
	annotations[aisConst.AISWSDockerImage] = consts.ConfigMap.WSSpec.DockerDaemonImage
	annotations[aisConst.AISWSDockerBIP] = consts.ConfigMap.WSSpec.DockerDaemonBIP
	annotations[aisConst.AISWSRuntime] = consts.ConfigMap.WSSpec.Runtime
	annotations[aisConst.AISWSGPUDeviceName] = consts.ConfigMap.WSSpec.GPUDeviceName
	if consts.ConfigMap.Harbor.AdminPullSecretName != "" {
		annotations[aisConst.AISWSImagePullSecretName] = consts.ConfigMap.Harbor.AdminPullSecretName
	}

	mews.Workspace.Name = ins.Name
	mews.Workspace.Namespace = ins.Namespace
	mews.Workspace.Spec.Image = ws.Image
	mews.Workspace.Spec.Labels = lbs
	mews.Workspace.Labels = lbs
	mews.Workspace.Spec.Annotations = annotations

	mews.Workspace.Spec.Envs = []corev1.EnvVar{
		{Name: "AISERVICE_WORKSPACE", Value: ws.ID.Hex()},
		{Name: "AISERVICE_INSTANCE", Value: ins.ID.Hex()},
		{Name: "AISERVICE_WORKSPACE_SERVER", Value: consts.ConfigMap.WSSpec.Service},
		{Name: "AISERVICE_WORKSPACE_BASE_URL", Value: fmt.Sprintf("%s/api/v1/lab/workspaces/%s/instances/%s/", consts.ConfigMap.WSAPIPrefix, ws.ID.Hex(), ins.ID.Hex())},
		{Name: "AISERVICE_WORKSPACE_TOKEN", Value: ins.Token},

		{Name: "DOMAIN", Value: consts.ConfigMap.Domain},
		{Name: "SAMPLE_DIR_PATH", Value: "/home/aiservice/data-augmentation-samples"},
		{Name: "UI_CONFIG_PATH", Value: "/aapi/publicservice.ais.io/api/v1/public/ui-configs"},

		{Name: "HARBOR_ENDPOINT", Value: consts.ConfigMap.Harbor.Endpoint},

		{Name: "AWS_ENDPOINT", Value: consts.ConfigMap.OSSConfig.Endpoint},
		{Name: "AWS_ACCESS_KEY_ID", Value: consts.ConfigMap.OSSConfig.AccessKeyID},
		{Name: "AWS_SECRET_ACCESS_KEY", Value: consts.ConfigMap.OSSConfig.AccessKeySecret},

		{Name: "NORI_CONTROLLER_ADDR", Value: consts.ConfigMap.NoriServer.ControllerAddr},
		{Name: "NORI_LOCATER_ADDR", Value: consts.ConfigMap.NoriServer.LocaterAddr},
	}

	var token string
	for _, tenant := range us.Tenants {
		if tenant.ID == ws.TenantID && tenant.HarborUser != nil {
			token = tenant.HarborUser.Token
		}
	}
	mews.Workspace.Spec.Envs = append(mews.Workspace.Spec.Envs, corev1.EnvVar{Name: "HARBOR_TOKEN", Value: token})

	if taskClaim, ok := cb.Revisions[0].TaskClaims[aisConst.TaskTypeRootfs]; ok {
		var taskSpec codehubTypes.RootfsTaskSpec
		taskSpecBytes, _ := json.Marshal(taskClaim.TaskSpec)
		if err := json.Unmarshal(taskSpecBytes, &taskSpec); err == nil {
			for k, v := range taskSpec.Environment {
				mews.Workspace.Spec.Envs = append(mews.Workspace.Spec.Envs, corev1.EnvVar{Name: k, Value: v})
			}
		}
	}

	mews.Workspace.Spec.Ports = []corev1.ServicePort{
		{
			Name:     "lab",
			Port:     80,
			Protocol: corev1.ProtocolTCP,
			TargetPort: intstr.IntOrString{
				Type:   intstr.Int,
				IntVal: 80,
			},
		},
		{
			Name:     "vscode",
			Port:     9826,
			Protocol: corev1.ProtocolTCP,
			TargetPort: intstr.IntOrString{
				Type:   intstr.Int,
				IntVal: 9826,
			},
		},
	}

	mews.Workspace.Spec.InstanceID = ins.ID.Hex()
	mews.Workspace.Spec.WorkspaceID = ws.ID.Hex()

	// todo 后续要去掉
	if ws.Spec.CPU == "" {
		ws.Spec.CPU = consts.ConfigMap.WSSpec.CPU
	}
	if ws.Spec.GPU == "" {
		ws.Spec.GPU = consts.ConfigMap.WSSpec.GPU
	}
	if ws.Spec.MEM == "" {
		ws.Spec.MEM = consts.ConfigMap.WSSpec.MEM
	}
	if ws.Spec.Storage == "" {
		ws.Spec.Storage = consts.ConfigMap.WSSpec.Storage
	}

	limits := corev1.ResourceList{
		"cpu":               resource.MustParse(ws.Spec.CPU),
		"memory":            resource.MustParse(ws.Spec.MEM),
		"ephemeral-storage": resource.MustParse(ws.Spec.Storage),
	}
	if ws.Spec.GPU != "" {
		limits[aisConst.NVIDIAGPUResourceName] = resource.MustParse(ws.Spec.GPU)
	}

	mews.Workspace.Spec.Resources = corev1.ResourceRequirements{Limits: limits}
	mews.Workspace.Spec.Affinity = ajob.BuildAffinity(ws.Spec.GPUTypes)
	if len(ws.Spec.GPUTypes) > 0 {
		mews.Workspace.Spec.Labels[aisConst.AISQuotaGPUTypeLabel] = strings.Join(ws.Spec.GPUTypes, ",")
	}

	log.Info(mews.Workspace)

	if err := CreatePersistentVolumeClaim(ins.Name, ins.Namespace, lbs, annotations); err != nil && !k8sErrors.IsAlreadyExists(err) {
		log.WithError(err).Error("failed to checkPersistentVolumeClaim")
		return err
	}

	if err := consts.Clientsets.WSClient.Workspaces().Startup(&mews.Workspace, ins.Name, ins.Namespace); err != nil {
		log.WithError(err).Error("failed to Startup Workspaces")
		return err
	}

	return nil
}

// Shutdown 关闭工作台
func Shutdown(c *gin.Context) {
	gc := ginlib.NewGinContext(c)
	userID := c.GetString(consts.UserID)
	tenantID := c.GetString(consts.TenantID)
	workspaceID, err := primitive.ObjectIDFromHex(c.GetString(consts.WorkspaceID))
	if err != nil {
		log.WithError(err).Error("failed to ObjectIDFromHex")
		ctx.Error(c, err)
		return
	}
	instanceID, err := primitive.ObjectIDFromHex(c.GetString(consts.InstanceID))
	if err != nil {
		log.WithError(err).Error("failed to ObjectIDFromHex")
		ctx.Error(c, err)
		return
	}

	ins, err := workspace.GetInstanceByID(gc, instanceID)
	if err != nil {
		log.WithError(err).Error("failed to GetInstanceByID")
		ctx.Error(c, err)
		return
	}
	if ins.WorkspaceID != workspaceID {
		log.Error("params invalid")
		ctx.Error(c, errors.ErrParamsInvalid)
		return
	}

	ws, err := workspace.GetWorkspaceByID(gc, workspaceID)
	if err != nil {
		log.WithError(err).Error("failed to GetWorkspaceByID")
		ctx.Error(c, err)
		return
	}

	if ws == nil || tenantID != ws.TenantID {
		log.Error("params is invalid")
		ctx.Error(c, errors.ErrParamsInvalid)
		return
	}
	if _, err := checkUserAuthority(gc, userID, ws); err != nil {
		log.WithError(err).Error("failed to checkUserAuthority")
		ctx.Error(c, err)
		return
	}

	if err := consts.Clientsets.WSClient.Workspaces().Stop(ins.Name, ins.Namespace); err != nil {
		log.WithError(err).Error("failed to Stop Workspaces")
	}

	if err := workspace.SetInstanceState(gc, ins.ID, consts.InstanceStateCompleted, ""); err != nil {
		log.WithError(err).Error("failed to SetInstanceState")
		ctx.Error(c, err)
		return
	}

	ctx.Success(c, nil)
}

func getInstance(gc *ginlib.GinContext) (*workspace.Instance, error) {
	instanceID, err := primitive.ObjectIDFromHex(gc.GetString(consts.InstanceID))
	if err != nil {
		log.WithError(err).Error("failed to ObjectIDFromHex")
		return nil, err
	}

	ins, err := workspace.GetInstanceByID(gc, instanceID)
	if err != nil {
		log.WithError(err).Error("failed to GetInstanceByID")
		return nil, err
	}

	workspaceID, err := primitive.ObjectIDFromHex(gc.GetString(consts.WorkspaceID))
	if err != nil {
		log.WithError(err).Error("failed to ObjectIDFromHex")
		return nil, err
	}

	ws, err := workspace.GetWorkspaceByID(gc, workspaceID)
	if err != nil {
		log.WithError(err).Error("failed to GetWorkspaceByID")
		return nil, err
	}

	if ws == nil || ws.ID != ins.WorkspaceID || ins.Token != gc.GetString(consts.Token) {
		log.WithField("ws", ws).WithField("ins", ins).WithField(consts.Token, gc.GetString(consts.Token)).Error("params is invalid")
		return nil, errors.ErrParamsInvalid
	}

	return ins, nil
}

func GetInstance(c *gin.Context) {
	gc := ginlib.NewGinContext(c)

	ins, err := getInstance(gc)
	if err != nil {
		log.WithError(err).Error("failed to getInstance")
		ctx.Error(c, err)
		return
	}

	ins.UsedDuration = time.Now().Unix() - ins.CreatedAt

	if err := workspace.SetInstanceLastPingAt(gc, ins.ID, time.Now().Unix()); err != nil {
		log.WithError(err).Error("failed to SetInstanceLastPingAt")
	}

	ctx.Success(c, ins)
}

type Settings struct {
	ExitTimeout *int64 `json:"exitTimeout"`
}

func getWorkspace(gc *ginlib.GinContext) (*workspace.Workspace, error) {
	workspaceID, err := primitive.ObjectIDFromHex(gc.GetString(consts.WorkspaceID))
	if err != nil {
		log.WithError(err).Error("failed to ObjectIDFromHex")
		return nil, err
	}

	ins, err := workspace.GetRunningInstanceByWorkspaceIDAndToken(gc, workspaceID, gc.GetString(consts.Token))
	if err != nil {
		log.WithError(err).Error("failed to GetRunningInstanceByWorkspaceIDAndToken")
		return nil, err
	}

	ws, err := workspace.GetWorkspaceByID(gc, workspaceID)
	if err != nil {
		log.WithError(err).Error("failed to GetWorkspaceByID")
		return nil, err
	}

	if ws == nil {
		log.WithField("ws", ws).WithField("ins", ins).WithField(consts.Token, gc.GetString(consts.Token)).Error("params is invalid")
		return nil, errors.ErrParamsInvalid
	}

	return ws, nil
}

func GetSettings(c *gin.Context) {
	gc := ginlib.NewGinContext(c)

	ws, err := getWorkspace(gc)
	if err != nil {
		log.WithError(err).Error("failed to getWorkspace")
		ctx.Error(c, err)
		return
	}

	ctx.Success(c, &Settings{ExitTimeout: &ws.ExitTimeout})
}

func UpdateSettings(c *gin.Context) {
	gc := ginlib.NewGinContext(c)

	ws, err := getWorkspace(gc)
	if err != nil {
		log.WithError(err).Error("failed to getWorkspace")
		ctx.Error(c, err)
		return
	}

	var settings Settings
	if err = c.ShouldBind(&settings); err != nil {
		log.Error("params invalid")
		ctx.Error(c, err)
		return
	}

	right := *settings.ExitTimeout != consts.ExitTimeoutM10 &&
		*settings.ExitTimeout != consts.ExitTimeoutH1 &&
		*settings.ExitTimeout != consts.ExitTimeoutH2 &&
		*settings.ExitTimeout != consts.ExitTimeoutH10
	if right {
		log.Error("params invalid")
		ctx.Error(c, errors.ErrParamsInvalid)
		return
	}

	if err := workspace.UpdateWorkspaceExitTimeout(gc, ws.ID, *settings.ExitTimeout); err != nil {
		ctx.Error(c, err)
		return
	}

	ctx.Success(c, nil)
}

func Proxy(c *gin.Context) {
	r := c.Request
	w := c.Writer
	rctx := r.Context()

	if r.URL.Query().Get("token") != "" {
		cookie := http.Cookie{
			Name:  "aiservice-workspace-token",
			Value: r.URL.Query().Get("token"),
		}
		http.SetCookie(w, &cookie)
		r.Header.Del("Cookie")
		r.AddCookie(&cookie)
	}
	nc, err := r.Cookie("aiservice-workspace-token")
	if err != nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	token := nc.Value
	svcs, err := consts.Clientsets.ServiceLister.Services(corev1.NamespaceAll).List(labels.SelectorFromSet(labels.Set{aisConst.AISWSInstanceToken: token}))
	if err != nil || len(svcs) == 0 {
		log.WithError(err).WithField(aisConst.AISWSInstanceToken, token).Warn("failed to get svc by token label")
		if svcs, err = consts.Clientsets.ServiceLister.Services(corev1.NamespaceAll).List(labels.SelectorFromSet(labels.Set{consts.Token: token})); err != nil || len(svcs) == 0 {
			log.WithError(err).WithField(consts.Token, token).Warn("failed to get svc by token label")
			w.WriteHeader(http.StatusForbidden)
			return
		}
	}

	rctx = context.WithValue(rctx, consts.Host, fmt.Sprintf("%s:80", svcs[0].Spec.ClusterIP))
	r = r.WithContext(rctx)
	r.URL.Path = consts.ConfigMap.WSAPIPrefix + r.URL.Path

	httpProxy := &httputil.ReverseProxy{Director: directorService}
	wsProxy := &wsutil.ReverseProxy{Director: directorService}

	if wsutil.IsWebSocketRequest(r) {
		wsProxy.ServeHTTP(w, r)
	} else {
		httpProxy.ServeHTTP(w, r)
	}
}

func directorService(r *http.Request) {
	log.Infoln(r.URL.Path)

	if _, ok := r.Header["User-Agent"]; !ok {
		r.Header.Set("User-Agent", "")
	}

	if wsutil.IsWebSocketRequest(r) {
		r.URL.Scheme = "ws"
	} else {
		r.URL.Scheme = "http"
	}

	r.URL.Host = r.Context().Value(consts.Host).(string)
}

func VSProxy(c *gin.Context) {
	r := c.Request
	w := c.Writer
	rctx := r.Context()

	if r.URL.Query().Get("token") != "" {
		cookie := http.Cookie{
			Name:  "aiservice-workspace-token",
			Value: r.URL.Query().Get("token"),
		}
		http.SetCookie(w, &cookie)
		r.Header.Del("Cookie")
		r.AddCookie(&cookie)
	}
	nc, err := r.Cookie("aiservice-workspace-token")
	if err != nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	token := nc.Value
	svcs, err := consts.Clientsets.ServiceLister.Services(corev1.NamespaceAll).List(labels.SelectorFromSet(labels.Set{aisConst.AISWSInstanceToken: token}))
	if err != nil || len(svcs) == 0 {
		log.WithError(err).WithField(aisConst.AISWSInstanceToken, token).Warn("failed to get svc by token label")
		if svcs, err = consts.Clientsets.ServiceLister.Services(corev1.NamespaceAll).List(labels.SelectorFromSet(labels.Set{consts.Token: token})); err != nil || len(svcs) == 0 {
			log.WithError(err).WithField(consts.Token, token).Warn("failed to get svc by token label")
			w.WriteHeader(http.StatusForbidden)
			return
		}
	}

	rctx = context.WithValue(rctx, consts.Host, fmt.Sprintf("%s:9826", svcs[0].Spec.ClusterIP))
	r = r.WithContext(rctx)
	r.URL.Path = c.Param("path")

	httpProxy := &httputil.ReverseProxy{Director: directorService}
	wsProxy := &wsutil.ReverseProxy{Director: directorService}

	if wsutil.IsWebSocketRequest(r) {
		wsProxy.ServeHTTP(w, r)
	} else {
		httpProxy.ServeHTTP(w, r)
	}
}
