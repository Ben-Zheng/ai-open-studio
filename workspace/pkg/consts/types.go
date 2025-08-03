package consts

import (
	"os"

	"gopkg.in/yaml.v2"
	"k8s.io/client-go/kubernetes"
	corev1 "k8s.io/client-go/listers/core/v1"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/authlib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/mgolib"
	aisTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/workspace/pkg/apis/client"
)

// Config 是全局配置文件对象
type Config struct {
	Domain            string                    `json:"domain" yaml:"domain"`
	WSAPIPrefix       string                    `json:"wsAPIPrefix" yaml:"wsAPIPrefix"`
	WSSpec            *WSSpec                   `json:"wsSpec" yaml:"wsSpec"`
	EndPoint          *aisTypes.EndPoint        `json:"endpoint" yaml:"endpoint"`
	GinConf           *ginlib.GinConf           `json:"ginConf" yaml:"ginConf"`
	MgoConf           *mgolib.MgoConf           `json:"mgoConf" yaml:"mgoConf"`
	AuthConfig        *authlib.AuthConf         `json:"authConfig" yaml:"authConfig"`
	TimeoutExitEnable bool                      `json:"timeoutExitEnable" yaml:"timeoutExitEnable"`
	OSSConfig         *aisTypes.OSSConfig       `json:"ossConfig" yaml:"ossConfig"`
	Harbor            *aisTypes.Harbor          `json:"harbor" yaml:"harbor"`
	NoriServer        *aisTypes.NoriServer      `json:"noriServer" yaml:"noriServer"`
	MultiSiteConfig   *aisTypes.MultiSiteConfig `json:"multiSiteConfig" yaml:"multiSiteConfig"`
}

type WSSpec struct {
	Service           string `json:"service" yaml:"service"`
	CPU               string `json:"cpu" yaml:"cpu"`
	MEM               string `json:"mem" yaml:"mem"`
	GPU               string `json:"gpu" yaml:"gpu"`
	GPUDeviceName     string `json:"gpuDeviceName" yaml:"gpuDeviceName"`
	Storage           string `json:"storage" yaml:"storage"`
	Capacity          string `json:"capacity" yaml:"capacity"`
	StorageClassName  string `json:"storageClassName" yaml:"storageClassName"`
	DockerDaemonImage string `json:"dockerDaemonImage" yaml:"dockerDaemonImage"`
	DockerDaemonBIP   string `json:"dockerDaemonBIP" yaml:"dockerDaemonBIP"`
	Runtime           string `json:"runtime" yaml:"runtime"`
	SSHProxyPort      int    `json:"sshProxyPort" yaml:"sshProxyPort"`
}

type Clientset struct {
	K8sClient     *kubernetes.Clientset
	WSClient      *client.WorkspaceBetaClient
	ServiceLister corev1.ServiceLister
}

// LoadFromYAML 用于从一个 YAML 文件中加载配置文件
func (c *Config) LoadFromYAML(path string) error {
	yamlStr, readErr := os.ReadFile(path)
	if readErr != nil {
		return readErr
	}

	err := yaml.Unmarshal(yamlStr, &c)
	if err != nil {
		return err
	}

	return nil
}
