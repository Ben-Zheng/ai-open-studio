package quota

import (
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	aisTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
	aisUtils "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/utils"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/consts"
	resourceCtrl "go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/controller/resource"
)

func ConvertQuantityToInt64(detail map[corev1.ResourceName]resource.Quantity, resourceName corev1.ResourceName) int64 {
	quantity := detail[resourceName]
	return aisUtils.ConvertQuantityToInt64(&quantity)
}

func KubebrainResourcesense(tenant, project, module, device string) ([]any, int64, int64, error) {
	matched := consts.ConfigMap.KubebrainConfig.MatchQuotaGroups(tenant, project, module, []string{device}, false)
	nodeDetails, err := consts.KylinClient.NodeDetails(matched.Tenant, matched.Project, matched.QuotaGroup, device)
	if err != nil {
		return nil, 0, 0, err
	}
	var ret []any
	dh := aisTypes.NewDeviceHelper(device)
	var total, available int64
	for _, machine := range nodeDetails.Machines {
		if ConvertQuantityToInt64(machine.Total, dh.KubeResourceName()) > 0 {
			machine.Status.Images = nil
			ret = append(ret, machine)
		}
		total += ConvertQuantityToInt64(machine.Total, dh.KubeResourceName())
		available += ConvertQuantityToInt64(machine.Usable, dh.KubeResourceName())
	}
	log.Infof("find kube resource: %+v, total: %+v, available: %+v", dh.KubeResourceName(), total, available)
	return ret, total, available, nil
}

func KorokResourceSense(tenant, project, module, device string, tags []string) ([]any, int64, int64, error) {
	devices, err := consts.KorokClient.ListDevices(consts.ConfigMap.KorokConfig.Group, tags)
	if err != nil {
		return nil, 0, 0, err
	}

	var ret []any
	var total, available int64
	for _, device := range devices {
		if device.Online { // 设备在线(如果在线为0 即提示无可用设备)
			total++
		}
		if device.Online && !device.IsOccupied { // 设备可用(如果可用为0，即提示设备被占满)
			available++
		}
		ret = append(ret, device)
	}
	return ret, total, available, nil
}

func AISResourceSense(tenant, project string, device string) ([]any, int64, int64, error) {
	// 配额 + 机器资源 取最小值
	dh := aisTypes.NewDeviceHelper(device)
	gc := ginlib.NewMockGinContext()
	tenantQuota, err := GetTenantDetail(gc, tenant)
	if err != nil {
		return nil, 0, 0, err
	}
	availableQuota := tenantQuota.TotalQuota.Sub(tenantQuota.ChargedQuota).NoNegative()
	quotaAvailable := ConvertQuantityToInt64(availableQuota, dh.KubeResourceName())

	_, nodes, err := resourceCtrl.ListNode(gc, "", "", true, true, false, nil)
	if err != nil {
		log.WithError(err).Error("failed to ListNode")
		return nil, 0, 0, err
	}

	var ret []any
	nodeUsable := 0.0
	nodeTotal := 0.0
	log.Infof("find total nodes: %+v", len(nodes))
	for i := range nodes {
		node := nodes[i]
		for _, label := range dh.Device.Labels {
			if node.GPUType == label {
				nodeTotal += node.GPU.Total
				nodeUsable += node.GPU.Usable
				ret = append(ret, node)
				break
			}
		}
	}

	log.Infof("ais resourcesense: %+v, %+v, %+v", nodeTotal, quotaAvailable, nodeUsable)
	if quotaAvailable > int64(nodeUsable) {
		return ret, int64(nodeTotal), int64(nodeUsable), nil
	}
	return ret, int64(nodeTotal), quotaAvailable, nil
}
