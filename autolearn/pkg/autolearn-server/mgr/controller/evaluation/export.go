package evaluation

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/config"
	autoLearnMgr "go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/mgr/controller/autolearn"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/mgr/dao"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/types"
	evalTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/evalhub/pkg/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/mgolib"
	aisTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/utils"
)

const MarkdownTmpl = `
# 实验对比报告

## 实验设置

|序号  | 实验任务 | 训练集地址 | 验证集地址 | Solution |
|---|---|---|---|---|---|{{range .ComparisonWithSnapshot.Comparison.Jobs}}
{{$evalJob := index $.JobIDToEvalJob .ID.Hex}}| {{.JobLabel}} |  {{formatRevision $evalJob }} | {{formatTrainDataset $evalJob $evalJob.Datasets }} | {{formatValidDataset $evalJob $evalJob.Datasets }} | {{formatAlg $evalJob}} |{{end}}

{{if .ComparisonWithSnapshot.DatasetGroups}}
## 数据集维度对比
{{range $group := .ComparisonWithSnapshot.DatasetGroups}}

### 数据集: {{formatEvalDataset $group $.JobIDToEvalJob }}

|序号 | 实验名称 | 评测子任务名称 | 模型 | {{range $metas := $group.MetricMetas}} {{showMetricCNName $metas.Name.Globalization}} | {{end}}
|---|---|---|---|{{range $group.MetricMetas}}---| {{end}}{{range $idx, $row := $group.TableRows}}
{{$evalJob := index $.JobIDToEvalJob .Job.ID.Hex}}|{{$idx}} | {{formatRevision $evalJob}}|  {{formatEvalJob $evalJob}} | model-{{$evalJob.IterationNumber}}.zip | {{range $metas := $group.MetricMetas}} {{showMetricValue $group.TableRows $idx $metas.Key}} | {{end}}{{end}}

{{end}}
{{end}}
`

type EvalComparisonExportData struct {
	EvalComparison         *types.EvalComparison
	JobIDToEvalJob         map[string]*types.ComparisonEvalJob
	ComparisonWithSnapshot *evalTypes.ComparisonWithSnapshot
}

func fillEvalComparison(d *dao.DAO, project string, jobIDToEvalJob map[string]*types.ComparisonEvalJob, autolearnIDs, revisionIDs []string) error {
	autolearns, err := dao.GetAutoLearnByAutoLearnIDs(d, project, autolearnIDs)
	if err != nil {
		return err
	}

	rvIDToAlgorithm, err := dao.FindAlgorithmByRevisionIDs(d, revisionIDs, false)
	if err != nil {
		return err
	}

	autolearnMap := make(map[string]*types.AutoLearn)
	revisionMap := make(map[string]*types.AutoLearnRevision)
	for _, autolearn := range autolearns {
		autolearnMap[autolearn.AutoLearnID] = autolearn
		for _, revision := range autolearn.AutoLearnRevisions {
			autoLearnMgr.TransformRevision(revision)

			if alg, ok := rvIDToAlgorithm[revision.RevisionID]; ok {
				var algorithms []*types.Algorithm
				for _, item := range alg.ActualAlgorithms {
					if item != nil {
						algorithms = append(algorithms, item)
					}
				}
				revision.Algorithms = algorithms
			}

			revisionMap[revision.RevisionID] = revision
		}
	}

	for _, evalJob := range jobIDToEvalJob {
		if rv, ok := revisionMap[evalJob.RevisionID]; ok {
			evalJob.Tenant = autolearnMap[evalJob.AutoLearnID].TenantID
			evalJob.Project = autolearnMap[evalJob.AutoLearnID].ProjectID
			evalJob.Algorithms = rv.Algorithms
			evalJob.Datasets = rv.Datasets
		}
	}
	return nil
}

func GenerateMakrdown(c *mgolib.MgoClient, cs *EvalComparisonExportData) ([]byte, error) {
	cs.JobIDToEvalJob = make(map[string]*types.ComparisonEvalJob)
	autolearnIDs := make([]string, 0)
	revisionIDs := make([]string, 0)
	d := dao.NewDAO(c, cs.ComparisonWithSnapshot.Comparison.Tenant)
	for _, item := range cs.EvalComparison.EvalJobs {
		cs.JobIDToEvalJob[item.EvaluationJobID] = item
		autolearnIDs = append(autolearnIDs, item.AutoLearnID)
		revisionIDs = append(revisionIDs, item.RevisionID)
	}
	if err := fillEvalComparison(d, cs.ComparisonWithSnapshot.Comparison.Project, cs.JobIDToEvalJob, autolearnIDs, revisionIDs); err != nil {
		return nil, err
	}

	tmpl, err := template.New("markdown").Funcs(template.FuncMap{
		"showMetricCNName": func(globalization *map[evalTypes.Language]string) string {
			if globalization == nil {
				return ""
			}
			return (*globalization)[evalTypes.LanguageCN]
		},
		"showMetricValue": func(rows []*evalTypes.ComparisonDataTableRow, idx int, key string) string {
			baseValues := rows[0].Values
			values := rows[idx].Values

			if values == nil {
				return "--"
			}
			val, ok := values[key]
			if !ok || val == nil {
				return "--"
			}

			if idx == 0 {
				return fmt.Sprintf("%v", val)
			}

			b, _ := utils.GetFloat64(baseValues[key])
			c, _ := utils.GetFloat64(val)
			return fmt.Sprintf("%v(%.5f)", val, c-b)
		},
		"formatTrainDataset": func(evalJob *types.ComparisonEvalJob, datasets []*types.Dataset) string {
			var trainDataset *types.Dataset
			for _, d := range datasets {
				if d.Type == aisTypes.DatasetTypeTrain {
					trainDataset = d
					break
				}
			}
			return formatDataset(evalJob, trainDataset)
		},
		"formatValidDataset": formatValidDataset,
		"formatRevision": func(evalJob *types.ComparisonEvalJob) string {
			return fmt.Sprintf("[%s/%s/%d](%s)", evalJob.AutoLearnName, evalJob.RevisionName, evalJob.IterationNumber,
				formatURI(fmt.Sprintf("ais/%s/automaticLearning/version/%s/%s?taskName=%s&versionName=%s&currentTab=profile",
					evalJob.Project, evalJob.AutoLearnID, evalJob.RevisionID, evalJob.AutoLearnName, evalJob.RevisionName)))
		},
		"formatEvalJob": func(evalJob *types.ComparisonEvalJob) string {
			// FIXME: 这里固定写死的检测评测路由
			return fmt.Sprintf("[%s](%s)", evalJob.EvaluationRJobName,
				formatURI(fmt.Sprintf("ais/%s/evalaute/%s/%s/result", evalJob.Project, evalJob.EvaluationID, evalJob.EvaluationJobID)))
		},
		"formatAlg": func(evalJob *types.ComparisonEvalJob) string {
			name := ""
			for idx, alg := range evalJob.Algorithms {
				if idx != 0 {
					name += " | "
				}
				name += alg.Name
			}
			return name
		},
		"formatEvalDataset": func(table *evalTypes.ComparisonDataTable, jobIDToEvalJob map[string]*types.ComparisonEvalJob) string {
			name := table.Dataset.DatasetName
			if len(table.TableRows) == 0 {
				return name
			}

			evalJob := jobIDToEvalJob[table.TableRows[0].Job.ID.Hex()]
			if evalJob == nil {
				return name
			}

			return formatValidDataset(evalJob, evalJob.Datasets)
		},
	}).Parse(MarkdownTmpl)
	if err != nil {
		return nil, err
	}
	content := bytes.NewBuffer([]byte{})
	err = tmpl.Execute(content, cs)
	if err != nil {
		return nil, err
	}
	return content.Bytes(), nil
}

// TODO: 如果前端路由规则变了， 这里需要跟着变化
func formatURI(path string) string {
	return fmt.Sprintf("https://%s/%s",
		strings.Trim(config.Profile.GlooGateway, "/"),
		strings.Trim(path, "/"))
}

func formatDataset(evalJob *types.ComparisonEvalJob, dataset *types.Dataset) string {
	if dataset == nil {
		return "-"
	}
	if dataset.SourceType == types.SourceTypeSDS {
		return dataset.SDSPath
	} else if dataset.SourceType == types.SourceTypeDataset {
		return fmt.Sprintf("[%s/%s](%s)", dataset.DatasetMeta.Name, dataset.DatasetMeta.RevisionName,
			formatURI(fmt.Sprintf("ais/%s/dataset/%s/%s/%s", evalJob.Project, dataset.DatasetMeta.OriginLevel, dataset.DatasetMeta.ID, dataset.DatasetMeta.RevisionID)))
	} else if dataset.SourceType == types.SourceTypePair {
		return fmt.Sprintf("[%s/%s](%s)", dataset.DatasetMeta.PairName, dataset.DatasetMeta.PairRevisionName,
			formatURI(fmt.Sprintf("ais/%s/trainValidation/%s/%s", evalJob.Project, dataset.DatasetMeta.PairID, dataset.DatasetMeta.PairRevisionID)))
	}
	return ""
}

func formatValidDataset(evalJob *types.ComparisonEvalJob, datasets []*types.Dataset) string {
	var validationDataset *types.Dataset
	for _, d := range datasets {
		if d.Type == aisTypes.DatasetTypeValid {
			validationDataset = d
			break
		}
	}
	return formatDataset(evalJob, validationDataset)
}
