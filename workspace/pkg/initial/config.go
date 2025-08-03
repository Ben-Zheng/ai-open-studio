package initial

import (
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/workspace/pkg/consts"
)

// InitConfig 从特定本地文件加载全局配置文件
func InitConfig(filepath string) {
	if err := consts.ConfigMap.LoadFromYAML(filepath); err != nil {
		panic(err)
	}
}
