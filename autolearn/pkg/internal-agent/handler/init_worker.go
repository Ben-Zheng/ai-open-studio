package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/internal-agent/oss"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/internal-agent/thread"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/recommend-server/api"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/utils"
	aisConst "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
)

type AlgorithmSource struct {
	Params       map[string]any    `json:"params"`
	Property     any               `json:"property"`
	DisplayInfos any               `json:"display_infos"`
	Envs         map[string]string `json:"envs,omitempty"`
	Resources    ResourceConfig    `json:"resources"`
}

type ResourceConfig struct {
	Train     ResourceDetail `json:"train"`
	Inference ResourceDetail `json:"inference"`
}

type ResourceDetail struct {
	CPU          int     `json:"cpu"`
	Memory       float64 `json:"memory"`
	GPU          int     `json:"gpu"`
	GPUName      string  `json:"gpu_name"`
	GPUTypeKey   string  `json:"gpu_type_key"`
	ResourceName string  `json:"resource_name"`
}

type InitWorker struct {
	Autolearn *thread.InternalAutoLearnRevision
	S3Client  *oss.S3Base
}

func NewInitWorker(autolearn *thread.InternalAutoLearnRevision) *InitWorker {
	awsEndpoint := os.Getenv(oss.OSS_ENDPOINT)
	awsAccesskey := os.Getenv(oss.AWS_ACCESS_KEY_ID)
	awsSecertKey := os.Getenv(oss.AWS_SECRET_ACCESS_KEY)

	awsConfig := oss.AwsConfig{
		Endpoint:        awsEndpoint,
		AccessKeyID:     awsAccesskey,
		AccessKeySecret: awsSecertKey,
	}

	s3client := oss.NewS3Base(&awsConfig)

	return &InitWorker{
		Autolearn: autolearn,
		S3Client:  s3client,
	}
}

func (m *InitWorker) Start(wg *sync.WaitGroup) {
	dur, err := time.ParseDuration(fmt.Sprint(m.Autolearn.TimeLimit) + "h")
	if err != nil {
		dur, _ = time.ParseDuration("300s")
	}

	var solutionConfigS3Path string
	if _, err := os.Stat(utils.RECOMMEND_SOLUTION_FILE); os.IsNotExist(err) {
		if _, err := os.Stat(utils.SN_OUTPUT_DIR); os.IsNotExist(err) {
			err = os.MkdirAll(utils.SN_OUTPUT_DIR, 0755)
			if err != nil {
				log.WithError(err).Error("[agent] create dir failed")
			}
		}

		solutionConfigS3Path, _ = makeAlgorithms(m)
	}

	var trainDatasets, valDatasets []string
	for _, dataset := range m.Autolearn.Datasets {
		if !checkDatasetPermssion(m, dataset.SDSPath) {
			thread.ReportPodState(types.ActionMasterPodCheckFailed, 401)
			return
		}
		if dataset.Type == aisConst.DatasetTypeTrain {
			trainDatasets = append(trainDatasets, dataset.SDSPath)
		}
		if dataset.Type == aisConst.DatasetTypeValid {
			valDatasets = append(valDatasets, dataset.SDSPath)
		}
	}

	mainCmd := "snapx"
	trainType := strings.ToLower(string(aisConst.AppTypeDetection))
	params := map[string]string{}
	params["time-limit"] = fmt.Sprint(dur.Seconds())
	params["check-datasets"] = ""

	if !m.Autolearn.AutoLearnRevision.Continued {
		params["export-format"] = m.Autolearn.AutoLearnRevision.ModelType
	}

	// 采样学习参数
	if m.Autolearn.AutoLearnRevision.SnapSampler != nil {
		samplerFile, err := downloadSampler(m)
		if err != nil {
			log.WithError(err).Error("[agent] create sampler file failed")
		} else {
			params["sampler-file"] = samplerFile
		}
	}

	if m.Autolearn.AutoLearnRevision.Continued {
		params["continue-from"] = m.Autolearn.AutoLearnRevision.ContinuedMeta.ModelPath
	}

	// KPS 和 Det 的 metrics 参数
	if m.Autolearn.AutoLearnRevision.Type == aisConst.AppTypeDetection ||
		m.Autolearn.AutoLearnRevision.Type == aisConst.AppTypeKPS ||
		m.Autolearn.AutoLearnRevision.Type == aisConst.AppTypeSegmentation {
		params["metric-type"] = fmt.Sprintf("%s@%s", m.Autolearn.EvalMetric.Key, m.Autolearn.EvalMetric.Condition.Key)
		params["metric-params"] = fmt.Sprintf("iou_threshold=%s;%s_threshold=%s", fmt.Sprint(m.Autolearn.EvalMetric.IOU), m.Autolearn.EvalMetric.Condition.Key, fmt.Sprint(m.Autolearn.EvalMetric.Condition.Value))

		if m.Autolearn.AutoLearnRevision.Type == aisConst.AppTypeKPS {
			trainType = strings.ToLower(string(aisConst.SnapAppTypeKPS))
			params["metric-params"] = fmt.Sprintf("confidence_threshold=%s;distance_threshold=%s", fmt.Sprint(m.Autolearn.EvalMetric.Condition.Value), fmt.Sprint(m.Autolearn.EvalMetric.Distance))
		}

		// 分割参数
		if m.Autolearn.AutoLearnRevision.Type == aisConst.AppTypeSegmentation {
			trainType = strings.ToLower(string(aisConst.AppTypeSegmentation))
		}
	}

	// Clf 的参数
	if m.Autolearn.AutoLearnRevision.Type == aisConst.AppTypeClassification {
		trainType = strings.ToLower(string(aisConst.AppTypeClassification))
		if strings.Contains(string(m.Autolearn.ClfEvalMetric.Key), "@") {
			keys := strings.Split(string(m.Autolearn.ClfEvalMetric.Key), "@")
			params["metric-type"] = strings.Trim(strings.Replace(string(m.Autolearn.ClfEvalMetric.Key), "threshold", "", 1), " ")
			params["metric-params"] = fmt.Sprintf("%s-threshold", strings.Trim(strings.Replace(keys[1], "threshold", "", 1), " ")) + "=" + fmt.Sprint(m.Autolearn.ClfEvalMetric.Value)
		} else {
			params["metric-type"] = strings.Replace(string(m.Autolearn.ClfEvalMetric.Key), "mean_", "", 1)
			if m.Autolearn.ClfEvalMetric.Key != types.EvalMetricKeyAccuracy {
				params["metric-params"] = fmt.Sprintf("class_names=['%s']", strings.Join(m.Autolearn.ClfEvalMetric.Attributes, "','"))
			}
		}
	}

	// 回归 metrics 参数
	if m.Autolearn.AutoLearnRevision.Type == aisConst.AppTypeClassificationAndRegression ||
		m.Autolearn.AutoLearnRevision.Type == aisConst.AppTypeRegression {
		trainType = strings.ToLower(string(aisConst.AppTypeClassification))
		params["metric-type"] = string(m.Autolearn.ClfEvalMetric.Key)
		params["scorer-type"] = "naive"
	}

	// OCR 参数
	if m.Autolearn.AutoLearnRevision.Type == aisConst.AppTypeOCR {
		trainType = strings.ToLower(string(aisConst.AppTypeOCR))
		params["metric-type"] = string(m.Autolearn.EvalMetric.Key)
	}

	// LLM 参数
	if m.Autolearn.AutoLearnRevision.Type == aisConst.AppTypeText2Text || m.Autolearn.AutoLearnRevision.Type == aisConst.AppTypeImage2Text {
		trainType = "llm"
		switch string(m.Autolearn.EvalMetric.Key) {
		case "rouge-1":
			params["metric-type"] = "rouge"
			params["metric-params"] = "rouge_metric='1'"
		case "rouge-2":
			params["metric-type"] = "rouge"
			params["metric-params"] = "rouge_metric='2'"
		case "rouge-l":
			params["metric-type"] = "rouge"
			params["metric-params"] = "rouge_metric='l'"
		case "bleu-4":
			params["metric-type"] = "bleu"
			params["metric-params"] = "bleu_metric='4-gram'"
		case "accuracy":
			params["metric-type"] = "avg_acc_score"
		}
	}

	// 通用参数
	params["solution-limit"] = fmt.Sprint(m.Autolearn.Resource.WorkerNum)
	params["output-dir"] = output(m.Autolearn)
	params["solution-config"] = solutionConfigS3Path

	if m.Autolearn.SolutionType != types.SolutionTypeCustom {
		params["solution-source"] = "snapsolution"
	}
	params["mode"] = "ajob"

	thread.ReportPodState(types.ActionMasterPodRunning, 0)
	runSnapdet(params, mainCmd, trainType, trainDatasets, valDatasets)
}

func (m *InitWorker) Stop() {}

func runSnapdet(params map[string]string, mainCMD, trainType string, trainDatasets, valDatasets []string) {
	param := []string{"--type", trainType, "train"}
	for k, v := range params {
		param = append(param, fmt.Sprintf("--%s", k), v)
	}

	for _, trainDataset := range trainDatasets {
		param = append(param, "--train", trainDataset)
	}
	for _, valDataset := range valDatasets {
		param = append(param, "--val", valDataset)
	}

	cmd := exec.Command(mainCMD, param...)
	log.Infof("[agent] cmd: %v", cmd.Args)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		log.WithError(err).Errorf("[agent] cmd %v start failed", cmd.Args)
	}

	if err := cmd.Wait(); err != nil {
		log.WithError(err).Errorf("[agent] snapdet process %d exit with err/signal: %v\n", cmd.Process.Pid, err)
	}
}

func generateResouces(a *types.Algorithm) ResourceConfig {
	return ResourceConfig{
		Train: ResourceDetail{
			CPU:          a.ProposedResource.TrainRequestsResources.CPU,
			GPU:          a.ProposedResource.TrainRequestsResources.GPU,
			Memory:       a.ProposedResource.TrainRequestsResources.Memory,
			GPUName:      a.ProposedResource.TrainRequestsResources.GPUName,
			GPUTypeKey:   a.ProposedResource.TrainRequestsResources.GPUTypeKey,
			ResourceName: a.ProposedResource.TrainRequestsResources.ResouceName,
		},
		Inference: ResourceDetail{
			CPU:          a.ProposedResource.InferRequestsResources.CPU,
			GPU:          a.ProposedResource.InferRequestsResources.GPU,
			Memory:       a.ProposedResource.InferRequestsResources.Memory,
			GPUName:      a.ProposedResource.InferRequestsResources.GPUName,
			GPUTypeKey:   a.ProposedResource.InferRequestsResources.GPUTypeKey,
			ResourceName: a.ProposedResource.InferRequestsResources.ResouceName,
		},
	}
}

func downloadAlgorithms(m *InitWorker) (string, error) {
	algorithms := m.Autolearn.Algorithms

	f, err := os.Create(utils.RECOMMEND_SOLUTION_FILE)
	if err != nil {
		log.WithError(err).Error("[agent] create recommend.soloution-config file failed")
	}
	defer f.Close()
	for _, a := range algorithms {
		var originFormatStr string
		var algorithm AlgorithmSource
		if err := json.Unmarshal([]byte(a.OriginalInfo), &algorithm); err != nil {
			log.WithError(err).Error("unmarshal algorithm failed")
			return "", err
		}

		for _, hp := range a.HyperParam {
			for key := range algorithm.Params {
				if key == hp.SnapdetKey {
					algorithm.Params[key] = hp.Value
				}
			}
		}

		algorithm.Resources = generateResouces(a)

		originFormatBytes, err := json.Marshal(algorithm)
		if err != nil {
			log.WithError(err).Error("unmarshal algorithm failed")
			return "", err
		}
		originFormatStr = bytes.NewBuffer(originFormatBytes).String()

		_, err = fmt.Fprintln(f, originFormatStr)
		if err != nil {
			log.WithError(err).Error("[agent] fprintln err when write files")
		}
	}
	// S3 cache recommend soltuion configs
	s3Path := recommendS3PATH(m.Autolearn)
	if _, err := m.S3Client.Upload(utils.RECOMMEND_SOLUTION_FILE, s3Path); err != nil {
		log.WithError(err).Error("[agent] failed to upload solution configs to S3")
	}
	return s3Path, nil
}

func makeAlgorithms(m *InitWorker) (string, error) {
	switch m.Autolearn.SolutionType {
	case types.SolutionTypePlatform, "":
		s3Path, err := downloadAlgorithms(m)
		if err != nil {
			return "", err
		}
		return s3Path, nil

	case types.SolutionTypeCustom:
		s3Path := generateAlgorithms(m)
		return s3Path, nil
	}
	return "", fmt.Errorf("[agent] make algorithms failed")
}

func generateAlgorithms(m *InitWorker) string {
	algorithms := m.Autolearn.Algorithms
	f, err := os.Create(utils.RECOMMEND_SOLUTION_FILE)
	if err != nil {
		log.WithError(err).Error("[agent] create recommend.soloution-config file failed")
	}
	defer f.Close()

	for _, a := range algorithms {
		envsString, err := json.Marshal(a.Envs)
		resourcesString, _ := json.Marshal(generateResouces(a))
		solName := a.ID
		if name, ok := a.Envs["SOLUTION_NAME"]; ok {
			solName = name.(string)
		}

		if err != nil {
			log.WithError(err).Error("strinfiy envs failed")
		}
		contentString := fmt.Sprintf(`{"property": {"id": %q, "name": %q, "version":"1.0.0"},"params": {}, "image": %q, "envs": %s, "resources": %s}`, a.ID, solName, a.ImageURI, string(envsString), string(resourcesString))
		_, err = fmt.Fprintln(f, contentString)
		if err != nil {
			log.WithError(err).Error("[agent] fprintln err when write files")
		}
	}

	// S3 cache recommend soltuion configs
	s3Path := recommendS3PATH(m.Autolearn)
	if _, err := m.S3Client.Upload(utils.RECOMMEND_SOLUTION_FILE, s3Path); err != nil {
		log.WithError(err).Error("[agent] failed to upload solution configs to S3")
	}

	return s3Path
}

func downloadSampler(m *InitWorker) (string, error) {
	sampler := m.Autolearn.AutoLearnRevision.SnapSampler
	dataSamplerBytes, err := json.Marshal(api.LoadSnapSampler(sampler))
	if err != nil {
		log.WithError(err).Error("failed to Marshal Sampler")
		return "", err
	}
	dataSamplerStr := bytes.NewBuffer(dataSamplerBytes).String()
	log.Info("sampler: ", dataSamplerStr)

	f, err := os.Create(utils.SNAP_SAMPLER_FILE)
	if err != nil {
		log.WithError(err).Error("[agent] create sampler.json file failed")
		return "", err
	}

	if _, err = fmt.Fprintln(f, dataSamplerStr); err != nil {
		log.WithError(err).Error("[agent] fprintln err when write files")
		return "", err
	}

	defer f.Close()
	return utils.SNAP_SAMPLER_FILE, nil
}

func downloadModelAndUnzip(m *InitWorker) (string, error) {
	modelS3Path := m.Autolearn.AutoLearnRevision.ContinuedMeta.ModelPath
	if err := os.MkdirAll(utils.CONTINUED_LEARN_MODEL_DIR, os.ModePerm); err != nil {
		return "", err
	}

	modelPath := fmt.Sprintf("%s/%s", utils.CONTINUED_LEARN_MODEL_DIR, utils.MODEL_DOWNLOAD_NAME)
	if err := m.S3Client.Download(modelS3Path, modelPath); err != nil {
		return "", err
	}

	if err := utils.Unzip(modelPath, utils.CONTINUED_LEARN_MODEL_DIR); err != nil {
		return "", err
	}

	var modelFilePath string
	if m.Autolearn.AutoLearnRevision.Type == aisConst.AppTypeClassification {
		modelFilePath = fmt.Sprintf("%s/%s", utils.CONTINUED_LEARN_MODEL_DIR, utils.CLF_TRAINED_MODEL_NAME)
	}

	if m.Autolearn.AutoLearnRevision.Type == aisConst.AppTypeDetection {
		modelFilePath = fmt.Sprintf("%s/%s", utils.CONTINUED_LEARN_MODEL_DIR, utils.DET_TRAINED_MODEL_NAME)
	}

	return modelFilePath, nil
}

func checkDatasetPermssion(m *InitWorker, s3Path string) bool {
	if err := m.S3Client.Head(s3Path); err != nil {
		log.WithError(err).Errorf("[agent] access s3 %s failed", s3Path)
		return false
	}
	return true
}

func output(autolearn *thread.InternalAutoLearnRevision) string {
	return fmt.Sprintf(
		utils.AUTOLEARN_S3_PREFIX,
		autolearn.Project,
		autolearn.AutoLearnID,
		autolearn.RevisionID,
	)
}

func recommendS3PATH(autolearn *thread.InternalAutoLearnRevision) string {
	return fmt.Sprintf(
		utils.AUTOLEARN_S3_PREFIX+"/recommend.solution-config",
		autolearn.Project,
		autolearn.AutoLearnID,
		autolearn.RevisionID,
	)
}
