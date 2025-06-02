package consts

import (
	"os"

	"gopkg.in/yaml.v2"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/mgolib"
	aisTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/utils/nori"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types/quota"
)

// Config 是全局配置文件对象
type Config struct {
	EndPoint             *aisTypes.EndPoint        `json:"endpoint" yaml:"endpoint"`
	GinConf              *ginlib.GinConf           `json:"ginConf" yaml:"ginConf"`
	MgoConf              *mgolib.MgoConf           `json:"mgoConf" yaml:"mgoConf"`
	AuthConf             *mgolib.MgoConf           `json:"authMgoConf" yaml:"authMgoConf"`
	NoriServer           *nori.Server              `json:"noriServer" yaml:"noriServer"`
	FluentbitServer      string                    `json:"fluentbitServer" yaml:"fluentbitServer"`
	KylinServer          string                    `json:"KylinServer" yaml:"KylinServer"`
	OSSConfig            *aisTypes.OSSConfig       `json:"ossConfig" yaml:"ossConfig"`
	KubebrainConfig      *aisTypes.KubebrainConfig `json:"kubebrainConfig" yaml:"kubebrainConfig"`
	PrometheusServer     string                    `json:"prometheusServer" yaml:"prometheusServer"`
	DiskExporterLabel    string                    `json:"diskExporterLabel" yaml:"diskExporterLabel"`
	DiskExporterPort     int64                     `json:"diskExporterPort" yaml:"diskExporterPort"`
	NodeLabels           []string                  `json:"nodeLabels" yaml:"nodeLabels"`
	PublicServer         string                    `json:"publicServer" yaml:"publicServer"`
	DatasetServer        string                    `json:"datasetServer" yaml:"datasetServer"`
	AuthServer           string                    `json:"authServer" yaml:"authServer"`
	StaticResourceBucket string                    `json:"staticResourceBucket" yaml:"staticResourceBucket"`
	Domain               string                    `json:"domain" yaml:"domain"`
	PublicAPIPrefix      string                    `json:"publicAPIPrefix" yaml:"publicAPIPrefix"`
	Harbor               aisTypes.Harbor           `json:"harbor" yaml:"harbor"`
	QuotaConfig          *QuotaConfig              `json:"quotaConfig" yaml:"quotaConfig"`
	KorokConfig          *aisTypes.KorokConfig     `json:"korokConfig" yaml:"korokConfig"`
	Loki                 *aisTypes.Loki            `json:"loki" yaml:"loki"`
	LogFrom              string                    `json:"logFrom" yaml:"logFrom" enums:"loki,es"`
	AccessToken          *aisTypes.AccessToken     `json:"accessToken" yaml:"accessToken"`
	KubebrainAccessToken *aisTypes.AccessToken     `json:"kubebrainAccessToken" yaml:"kubebrainAccessToken"`
	MonitorEnabled       bool                      `json:"monitorEnabled" yaml:"monitorEnabled"`
	MultiSiteConfig      *aisTypes.MultiSiteConfig `json:"multiSiteConfig" yaml:"multiSiteConfig"`
	APUConfigs           []*aisTypes.APUConfig     `json:"apuConfigs" yaml:"apuConfigs"`
}

type QuotaConfig struct {
	InflationRate    uint16          `json:"inflationRate" yaml:"inflationRate"`       // 超发率(默认 100, 不超发)
	FixedQuota       quota.QuotaData `json:"fixedQuota" yaml:"fixedQuota"`             // 可选的手动配置集群总配额
	IgnoreSystemUsed bool            `json:"ignoreSystemUsed" yaml:"ignoreSystemUsed"` // 是否忽略系统占用统计
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
