package initial

import (
	log "github.com/sirupsen/logrus"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/features"
	aisTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/consts"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types"
)

// InitConfig 从特定本地文件加载全局配置文件
func InitConfig(filepath string) {
	err := consts.ConfigMap.LoadFromYAML(filepath)
	if err != nil {
		panic(err)
	}
	if consts.ConfigMap.QuotaConfig == nil {
		consts.ConfigMap.QuotaConfig = &consts.QuotaConfig{
			InflationRate: 100,
		}
	}

	if features.IsKubebrainRuntime() && consts.ConfigMap.KylinServer == "" {
		consts.ConfigMap.KylinServer = "https://hh-d.brainpp.cn"
		if consts.ConfigMap.KubebrainAccessToken == nil {
			consts.ConfigMap.KubebrainAccessToken = &aisTypes.AccessToken{}
		}
	}

	if consts.ConfigMap.KubebrainConfig != nil {
		consts.ConfigMap.KubebrainConfig.Init()
	}

	for i := range consts.ConfigMap.KubebrainConfig.ExtraQuotaGroups {
		log.Infof("kubebrain extra quota group: %s->%s", consts.ConfigMap.KubebrainConfig.ExtraQuotaGroups[i].Cond, consts.ConfigMap.KubebrainConfig.ExtraQuotaGroups[i].Info)
	}

	if consts.ConfigMap.QuotaConfig.FixedQuota != nil {
		for k := range consts.ConfigMap.QuotaConfig.FixedQuota {
			if types.K8sResourceNameMap[k] {
				continue
			}
			consts.ConfigMap.QuotaConfig.FixedQuota[k] = 0
		}
	}
}
