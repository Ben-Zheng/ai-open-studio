package site

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	kubegatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1/kube/apis/gateway.solo.io/v1"
	gwclientset "github.com/solo-io/gloo/projects/gateway/pkg/api/v1/kube/client/clientset/versioned"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	glooextauthv1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/ssl"
	gloocore "github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	skutils "github.com/solo-io/solo-kit/pkg/utils/kubeutils"
	"google.golang.org/protobuf/types/known/wrapperspb"
	corev1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	coreInformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	coreListers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"

	authClientv1 "go.megvii-inc.com/brain/brainpp/projects/aiservice/auth/pkg/api/v1"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/features"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/oss"
	aisTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/consts"
	siteTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types/site"
)

const (
	siteReportInterval     int64 = 10              // 每隔 10s 遍历处理未结束清洗任务
	periodSyncAuthInterval       = time.Second * 5 // 同步 namespace 的 quota label
)

type ReportPayload struct {
	SiteInfo  *siteTypes.Site
	Timestamp int64
}

// 定期向中央集群上报自己的状态
func PeriodReportSiteStatus() {
	ticker := time.NewTicker(time.Duration(siteReportInterval) * time.Second)
	log.Infof("PeriodReportSiteStatus started")

	self := &siteTypes.Site{
		Name:        consts.ConfigMap.MultiSiteConfig.Site,
		DisplayName: consts.ConfigMap.MultiSiteConfig.Name,
		IsCenter:    consts.ConfigMap.MultiSiteConfig.IsCenter,
		Domain:      consts.ConfigMap.MultiSiteConfig.Domain,
		AdminDomain: consts.ConfigMap.MultiSiteConfig.AdminDomain,
	}
	center := &siteTypes.Site{
		Domain: consts.ConfigMap.MultiSiteConfig.CenterDomain,
	}
	client := NewClient(nil)

	report := func() {
		gc := ginlib.NewMockGinContext()
		gc.SetAuthorization(fmt.Sprintf("Basic %s",
			base64.StdEncoding.EncodeToString([]byte(consts.ConfigMap.MultiSiteConfig.CenterAK+":"+consts.ConfigMap.MultiSiteConfig.CenterSK))))
		if err := client.ReportStatus(gc, self, center); err != nil {
			log.WithError(err).Errorf("fuck report status...")
		}
	}
	report()
	for {
		for range ticker.C {
			log.Infof("PeriodReportSiteStatus running")
			report()
		}
	}
}

func OnSiteJoin(ctx context.Context, site *siteTypes.Site) error {
	// 1. 创建 gloo entry vs
	// 2. 增加 coredns 域名映射 （可选）
	if site.IsCenter {
		log.Infof("ignore center site join!")
		return nil
	}
	restConfig, err := rest.InClusterConfig()
	if err != nil {
		log.WithError(err).Error("failed to InClusterConfig")
		panic(err)
	}

	aiserviceName, aisadminName := siteGlooVirtualServiceName(site)
	gatewayClientSet, err := gwclientset.NewForConfig(restConfig)
	if err != nil {
		return err
	}

	if _, err := gatewayClientSet.GatewayV1().VirtualServices("aiservice-gateway").Get(ctx, aiserviceName, metav1.GetOptions{}); err != nil {
		if k8sErrors.IsNotFound(err) {
			if err := createGlooVirtualService(ctx, gatewayClientSet, aiserviceName, site.Domain, site); err != nil {
				return err
			}
		} else {
			log.WithError(err).Errorf("failed to get gloo virtualservice: %s", aiserviceName)
		}
	} else {
		log.Infof("gloo virtualservice: %s already exist!", aiserviceName)
	}

	if _, err := gatewayClientSet.GatewayV1().VirtualServices("aiservice-gateway").Get(ctx, aisadminName, metav1.GetOptions{}); err != nil {
		if k8sErrors.IsNotFound(err) {
			if err := createGlooVirtualService(ctx, gatewayClientSet, aisadminName, site.AdminDomain, site); err != nil {
				return err
			}
		} else {
			log.WithError(err).Errorf("failed to get gloo virtualservice: %s", aisadminName)
		}
	} else {
		log.Infof("gloo virtualservice: %s already exist!", aisadminName)
	}
	return nil
}

func createGlooVirtualService(ctx context.Context, gatewayClientSet *gwclientset.Clientset, vsName, domain string, site *siteTypes.Site) error {
	useHTTPS := true
	domain = strings.TrimPrefix(domain, "https://")
	if strings.HasPrefix(domain, "http://") {
		useHTTPS = false
		domain = strings.TrimPrefix(domain, "http://")
	}
	// Create a virtual service pointing to the upstream
	vsMeta := &gloocore.Metadata{
		Name:      vsName,
		Namespace: "aiservice-gateway",
	}
	vs := &kubegatewayv1.VirtualService{
		ObjectMeta: skutils.ToKubeMeta(vsMeta),
		Spec: gatewayv1.VirtualService{
			Metadata: vsMeta,
			VirtualHost: &gatewayv1.VirtualHost{
				Domains: []string{
					domain,
				},
				Routes: []*gatewayv1.Route{
					{
						Matchers: []*matchers.Matcher{
							{
								PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/aapi/auth.ais.io/"},
							},
						},
						Options: &gloov1.RouteOptions{
							Extauth: &glooextauthv1.ExtAuthExtension{
								Spec: &glooextauthv1.ExtAuthExtension_CustomAuth{
									CustomAuth: &glooextauthv1.CustomAuth{
										ContextExtensions: map[string]string{},
									},
								},
							},
							PrefixRewrite: &wrapperspb.StringValue{Value: "/"},
						},
						Action: &gatewayv1.Route_RouteAction{
							RouteAction: &gloov1.RouteAction{
								Destination: &gloov1.RouteAction_Single{
									Single: &gloov1.Destination{
										DestinationType: &gloov1.Destination_Upstream{
											Upstream: &gloocore.ResourceRef{
												Name:      "aiservice-auth-server-http-8080",
												Namespace: "aiservice-gateway",
											},
										},
									},
								},
							},
						},
					},
					{
						Matchers: []*matchers.Matcher{
							{
								PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/aapi"},
							},
						},
						Action: &gatewayv1.Route_RouteAction{
							RouteAction: &gloov1.RouteAction{
								Destination: &gloov1.RouteAction_Single{
									Single: &gloov1.Destination{
										DestinationType: &gloov1.Destination_Upstream{
											Upstream: &gloocore.ResourceRef{
												Name:      "aiservice-auth-server-http-8080",
												Namespace: "aiservice-gateway",
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	if useHTTPS {
		vs.Spec.SslConfig = &ssl.SslConfig{
			Parameters: &ssl.SslParameters{
				CipherSuites: []string{
					"AES128-GCM-SHA256",
					"ECDHE-ECDSA-AES128-GCM-SHA256",
					"ECDHE-RSA-AES128-GCM-SHA256",
					"ECDHE-ECDSA-AES256-GCM-SHA384",
					"ECDHE-RSA-AES256-GCM-SHA384",
					"ECDHE-ECDSA-CHACHA20-POLY1305",
					"ECDHE-RSA-CHACHA20-POLY1305",
				},
				MinimumProtocolVersion: ssl.SslParameters_TLSv1_2,
			},
			SslSecrets: &ssl.SslConfig_SecretRef{
				SecretRef: &gloocore.ResourceRef{
					Name:      "aiservice-cert-secret",
					Namespace: "aiservice",
				},
			},
		}
	}

	if _, err := gatewayClientSet.GatewayV1().VirtualServices("aiservice-gateway").Create(ctx, vs, metav1.CreateOptions{}); err != nil {
		log.WithError(err).Errorf("failed to create gloo virtualservice: %s, %+v", vsName, vs)
		return err
	}
	return nil
}

func siteGlooVirtualServiceName(site *siteTypes.Site) (string, string) {
	return fmt.Sprintf("%s-aiservice", site.Name), fmt.Sprintf("%s-aisadmin", site.Name)
}

type AuthSideEffectSyncer struct {
	authClient      authClientv1.Client
	clientset       *kubernetes.Clientset
	namespaceLister coreListers.NamespaceLister
	namespaceSynced cache.InformerSynced
}

func NewAuthSideEffectSyncer(nsInformer coreInformers.NamespaceInformer, authClient authClientv1.Client, clientset *kubernetes.Clientset) *AuthSideEffectSyncer {
	return &AuthSideEffectSyncer{
		authClient:      authClient,
		clientset:       clientset,
		namespaceLister: nsInformer.Lister(),
		namespaceSynced: nsInformer.Informer().HasSynced,
	}
}

func (ns *AuthSideEffectSyncer) Run(stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	log.Info("[Multisite] Waiting for multisite caches to be synced")
	if ok := cache.WaitForCacheSync(stopCh, ns.namespaceSynced); !ok {
		log.Info("auth side effect syncer failed to wait for caches to sync")
		return
	}
	log.Info("auth side effect syncer starting...")

	if !features.IsMultiSiteEnabled() {
		log.Info("auth side effect syncer multi site feature isn't enabled!")
		return
	}

	ticker := time.NewTicker(periodSyncAuthInterval)
	defer ticker.Stop()
	for {
		for range ticker.C {
			_, tenants, err := ns.authClient.ListTenant(nil)
			if err != nil {
				log.WithError(err).Error("failed to list auth tenants")
				continue
			}
			for _, tenant := range tenants {
				projects, err := ns.authClient.ListProjectByTenant(nil, tenant.ID)
				if err != nil {
					log.WithError(err).Errorf("auth side effect syncer failed to list auth projects: %s", tenant.ID)
					continue
				}
				for _, project := range projects {
					if features.IsNamespacePerTenant() {
						namespace := features.GetProjectWorkloadNamespace(project.ID)
						_, err := ns.namespaceLister.Get(namespace)
						if err != nil {
							if k8sErrors.IsNotFound(err) {
								_, err := ns.clientset.CoreV1().Namespaces().Create(context.TODO(), &corev1.Namespace{
									TypeMeta: metav1.TypeMeta{},
									ObjectMeta: metav1.ObjectMeta{
										Labels: map[string]string{
											aisTypes.AISQuotaAdmissionNSLabel: "true",
											aisTypes.AISOSSIsolationNSLabel:   "true",
										},
										Name: namespace,
									},
									Spec: corev1.NamespaceSpec{},
								}, metav1.CreateOptions{})
								if err != nil {
									log.WithError(err).Errorf("auth side effect syncer failed to create namespace: %s", namespace)
									continue
								}
							}
							log.WithError(err).Errorf("auth side effect syncer failed to get namespace: %s/%s", tenant.ID, project.Name)
							continue
						}
					} else {
						log.Info("auth side effect syncer namespace per tenant feature isn't enabled!")
					}

					ossClient := oss.NewClientWithSession(consts.OSSSession)
					for _, bucketName := range oss.GetAllSystemBuckets(project.ID, tenant.ID) {
						exist, err := ossClient.CheckBucketExists(bucketName)
						if err != nil {
							log.WithError(err).Errorf("auth side effect syncer failed to check bucket exist: %s/%s, %s", tenant.ID, project.Name, bucketName)
							continue
						}
						if !exist {
							if err := ossClient.CreateBucket(bucketName); err != nil {
								log.WithError(err).Errorf("auth side effect syncer failed to create bucket: %s/%s, %s", tenant.ID, project.Name, bucketName)
								continue
							}
						}
					}
				}
			}
		}
	}
}
