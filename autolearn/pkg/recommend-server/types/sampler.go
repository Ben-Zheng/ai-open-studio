package types

import (
	autolearnTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/types"
)

type SnapSampler struct {
	SnapSamplerWeight
	AtomSamplers  []AtomSampler `json:"atom_samplers"`  // 原子采样器列表
	StatisticInfo StatisticInfo `json:"statistic_info"` // 采样权重更新后，整体数据的分布统计
}

type SnapSamplerWeight struct {
	SnapSamplerInput
	Weights map[string]float64 `json:"weights"` // 原子采样器的权重参数
}

type SnapSamplerInput struct {
	SDSPaths []string                  `json:"datasets_path"`  // sds 文件的绝对路径，需要和最终送到snapdet/clf 的顺序保持严格一致
	Method   autolearnTypes.MethodType `json:"sampler_method"` // 按数据集采样：bydataset， 按类别采样 byclass
	Task     autolearnTypes.TaskType   `json:"task"`           // 检测任务：det， 分类任务：clf
}

type AtomSampler struct {
	Name          string        `json:"name"`           // 采样器的名字
	Num           int64         `json:"num"`            // 采样器元素数量
	StatisticInfo StatisticInfo `json:"statistic_info"` // 采样器的基础统计信息
}

type StatisticInfo struct {
	DatasetInfo map[string]float64 `json:"dataset_info"` // 数据集相关的统计信息
	ClassInfo   map[string]float64 `json:"class_info"`   // 类别的统计信息
}
