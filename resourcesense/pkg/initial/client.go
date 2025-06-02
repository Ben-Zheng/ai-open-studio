package initial

import (
	datahubv1 "go.megvii-inc.com/brain/brainpp/projects/aiservice/datahub/pkg/datahub-apiserver/api/v1"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/features"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/korok"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/kylin"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/release"
	etcdUtils "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/utils/etcd"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/consts"
)

func InitClients(etcdCertPath string) {
	consts.EtcdClient = etcdUtils.InitEtcd(etcdCertPath)

	consts.ReleaseClient = release.NewClient(consts.ConfigMap.EndPoint)

	korokClient, err := korok.NewClient(consts.ConfigMap.KorokConfig.Address, consts.ConfigMap.KorokConfig.Token)
	if err != nil {
		panic(err)
	}
	consts.KorokClient = korokClient
	if features.IsKubebrainRuntime() {
		consts.KylinClient = kylin.NewKylinClient(consts.ConfigMap.KylinServer, "", consts.ConfigMap.KubebrainAccessToken.AccessKey, consts.ConfigMap.KubebrainAccessToken.SecretKey)
	}

	consts.DatahubClient = datahubv1.NewClient(consts.ConfigMap.EndPoint.DatahubAPIServer)
}
