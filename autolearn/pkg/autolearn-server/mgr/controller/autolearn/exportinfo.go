package autolearn

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
	"time"

	"gopkg.in/yaml.v3"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/types"
	aisTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
)

const ScoreMarkdown = `
## 算法分数对比
### 1 自动学习实验详情
{{- $e := .EvalMetric }}
| 任务名 | 实验名 | 训练集 | 验证集 | 算法个数 | 评测指标 | 最大分数值 |
| :---: | :---: | :---: | :---: | :---: | :---: | :---: |
| {{ .AutoLearnName }} | {{ .RevisionName }} | {{ .Train }} | {{ .Validation }} | {{ len .Algorithms }} | {{ $e.Key }}@iou={{ $e.IOU }}@{{ $e.Condition.Key }}={{ $e.Condition.Value }} | {{ $e.MaxScore }} |

### 2 每次迭代分数值
| 迭代版本号 | 算法名 | 分数 | 时间 |
| :---: | :---: | :---: | :---: |
{{- range $val := .BestDetector.BestDetectorScore }}
| {{ $val.Score.IterationNumber }} | {{ $val.AlgorithmName }} | {{ $val.Score.Value }} | {{ timeFormat $val.Score.Time }} |
{{- end }}

### 3 各算法分数值
{{- range $idx, $val := .Algorithms }}
> 算法{{ inc $idx }} {{ $val.Name }}

| 分数 | 迭代版本号 | 时间 |
| :---: | :---: | :---: |
{{- range $s := $val.Score }}
| {{ $s.Value }} | {{ if $s.IterationNumber }} {{ $s.IterationNumber }} {{ else }} - {{ end }} | {{ timeFormat $s.Time }} |
{{- end }}
{{ end }}
`

func AlgorithmScoreMarkdown(resp *types.ExportAlgorithmScoreResp) ([]byte, error) {
	tmpl, err := template.New("score").Funcs(template.FuncMap{
		"inc": func(i int) int {
			return i + 1
		},
		"timeFormat": func(t int64) string {
			return time.Unix(t, 0).Format("15:04:05")
		},
	}).Parse(ScoreMarkdown)
	if err != nil {
		return nil, err
	}

	content := bytes.NewBuffer([]byte{})
	if err := tmpl.Execute(content, resp); err != nil {
		return nil, err
	}

	return content.Bytes(), nil
}

type basicInfo struct {
	TaskName       string                  `yaml:"task_name"`
	Link           string                  `yaml:"link"` // 无需填写
	SolutionName   string                  `yaml:"solution_name"`
	TrainDataset   map[string]*datasetInfo `yaml:"train"`
	ValDataset     map[string]string       `yaml:"val"`
	ClassThreshold map[string]*classInfo   `yaml:"class_threshold"`
}

type datasetInfo struct {
	Path   string  `yaml:"path"`
	Weight float64 `yaml:"weight"`
}

type classInfo struct {
	HighThreshold string  `yaml:"high_threshold"` // 无需填写
	LowThreshold  string  `yaml:"low_threshold"`  // 无需填写
	Weights       float64 `yaml:"weights"`
}

func BasicInfoForYaml(rv *types.AutoLearnRevision) ([]byte, error) {
	res := basicInfo{
		TaskName:       rv.AutoLearnName + "/" + rv.RevisionName,
		ValDataset:     map[string]string{"val_000": ""},
		TrainDataset:   make(map[string]*datasetInfo),
		ClassThreshold: make(map[string]*classInfo),
	}

	rv.BestDetector = GetBestDetector(rv.Type, rv.Algorithms)
	if len(rv.BestDetector.BestDetectorScore) != 0 {
		res.SolutionName = rv.BestDetector.BestDetectorScore[len(rv.BestDetector.BestDetectorScore)-1].AlgorithmName
	}
	if res.SolutionName == "" && len(rv.Algorithms) != 0 {
		res.SolutionName = rv.Algorithms[0].Name
	}

	trainNum := 0
	for i := range rv.Datasets {
		if rv.Datasets[i].Type == aisTypes.DatasetTypeTrain && rv.SnapSampler == nil {
			res.TrainDataset[fmt.Sprintf("train_%03d", trainNum)] = &datasetInfo{
				Path:   rv.Datasets[i].SDSPath,
				Weight: -1,
			}
			trainNum++

			for j := range rv.Datasets[i].Classes {
				res.ClassThreshold[strings.TrimPrefix(rv.Datasets[i].Classes[j], "default.")] = &classInfo{
					Weights: -1,
				}
			}
		}
	}

	if rv.SnapSampler != nil {
		trainNum := 0
		for p, w := range rv.SnapSampler.StatisticInfo.DatasetInfo {
			if rv.SnapSampler.Method == types.MethodTypeByClass {
				w = -1
			}
			res.TrainDataset[fmt.Sprintf("train_%03d", trainNum)] = &datasetInfo{
				Path:   p,
				Weight: w,
			}
			trainNum++
		}
		for c, w := range rv.SnapSampler.StatisticInfo.ClassInfo {
			if rv.SnapSampler.Method == types.MethodTypeByDataset {
				w = -1
			}
			res.ClassThreshold[strings.TrimPrefix(c, "default.")] = &classInfo{
				Weights: w,
			}
		}
	}

	return yaml.Marshal(res)
}
