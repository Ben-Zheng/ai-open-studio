package types

import "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/utils/sds"

type OpenAIChatStreamResp struct {
	ID      string `json:"id"`
	Object  string `json:"object"`  // 固定字符
	Created int64  `json:"created"` // 时间戳
	Model   string `json:"model"`   // 模型名称

}

type OpenAIChatRespChoiceStream struct {
	ID                string     `json:"id"`
	Object            string     `json:"object"`
	Created           float64    `json:"created"`
	Model             string     `json:"model"`
	SystemFingerprint string     `json:"system_fingerprint"`
	Choices           []*Choices `json:"choices"`
}

type Choices struct {
	Index        int64        `json:"index"`
	Delta        *ChoiceDelta `json:"delta"`
	FinishReason string       `json:"finish_reason"`
}

type ChoiceDelta struct {
	Role    sds.Role `json:"role"`
	Content string   `json:"content"`
}

type DefaultResource struct {
	CVModelResource             []*Resource            `json:"cvModelResource"`             // 传统CV转换前模型的资源
	ConvertedCVModelResourceMap map[string][]*Resource `json:"convertedCVModelResourceMap"` // 转换后模型的资源
	AIGCModelResourceMap        map[string][]*Resource `json:"aigcModelResourceMap"`        // AIGC模型的资源
}
