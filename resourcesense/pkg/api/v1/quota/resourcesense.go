package quota

import (
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/features"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	aisTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
	quotaCtrl "go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/controller/quota"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types/ctx"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types/ctx/errors"
)

type NodeResourceSenseResp struct {
	Total      int64 `json:"total"`     // 所有资源数 (可调度)
	Available  int64 `json:"available"` // 可用资源数 (可申请)
	KubeNodes  []any `json:"kubeNodes"`
	KorokNodes []any `json:"korokNodes"`
	Nodes      []any `json:"nodes"`
}

func NodesResourceSense(c *gin.Context) {
	gc := ginlib.NewGinContext(c)
	device := c.Query("device")
	resourceType := c.Query("resourceType")
	korokTags := c.QueryArray("korokTags")
	withNodes := c.GetBool("withNodes")

	var devices []any
	var total, available int64
	var err error
	resp := &NodeResourceSenseResp{}
	dh := aisTypes.NewDeviceHelper(device)

	logger := log.WithFields(
		log.Fields{
			"device": device,
			"dh":     dh,
		},
	)
	if dh.IsUnknown() {
		logger.Error("unknown device")
		ctx.Error(c, errors.ErrParamsInvalid)
		return
	}

	if dh.IsEmbedded() {
		logger.Info("is embedded...")
		devices, total, available, err = quotaCtrl.KorokResourceSense(gc.GetAuthTenantID(), gc.GetAuthProjectID(), resourceType, device, korokTags)
		resp.KorokNodes = devices
	} else {
		if features.IsKubebrainRuntime() {
			logger.Info("is kubebrain...")
			devices, total, available, err = quotaCtrl.KubebrainResourcesense(gc.GetAuthTenantID(), gc.GetAuthProjectID(), resourceType, device)
			resp.KubeNodes = devices
		} else {
			logger.Info("is aiservice...")
			devices, total, available, err = quotaCtrl.AISResourceSense(gc.GetAuthTenantID(), gc.GetAuthProjectID(), device)
			resp.Nodes = devices
		}
	}
	if err != nil {
		log.WithError(err).Error("failed to resourcesense")
		ctx.Error(c, err)
		return
	}
	resp.Total = total
	resp.Available = available
	log.Infof("query resourcesense, device: %+v", *dh.Device)
	if !withNodes {
		resp.KorokNodes = nil
		resp.KubeNodes = nil
		resp.Nodes = nil
	}
	ctx.Success(c, resp)
}
