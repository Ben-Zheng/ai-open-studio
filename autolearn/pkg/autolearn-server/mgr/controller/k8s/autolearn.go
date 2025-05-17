package k8s

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/config"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/utils"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/features"
	aisConst "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
	publicTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/publicservice/pkg/types"
	gv1 "go.megvii-inc.com/brain/brainpp/projects/kubebrain/pkg/apis/gang/v1alpha1"
	schedulerv1alpha1 "go.megvii-inc.com/brain/brainpp/projects/kubebrain/pkg/client/clientset/versioned/typed/gang/v1alpha1"
)

func CreateAutoLearnMasterPod(brainclient *schedulerv1alpha1.SchedulerV1alpha1Client, kubeclient *kubernetes.Clientset, autoLearn *types.AutoLearn, revision *types.AutoLearnRevision) error {
	pod := buildAutoLearnPod(autoLearn, revision)
	if features.IsSupportGangSchedule() {
		podCount := getInt32Ptr(revision.Resource.WorkerNum * 2) // 每个 sol 2个runner
		dur, err := time.ParseDuration(fmt.Sprint(revision.ScheduleTimeLimit) + "h")
		if err != nil {
			log.WithError(err).Error("parse schedule timeout data failed")
		}
		podGroup := &gv1.PodGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pod.Name,
				Namespace: pod.Namespace,
			},
			Spec: gv1.PodGroupSpec{
				Replicas:       podCount,
				MinAvailable:   podCount,
				TimeoutSeconds: getInt32Ptr(int32(dur.Seconds())),
			},
		}
		_, err = brainclient.PodGroups(pod.Namespace).Create(context.TODO(), podGroup, metav1.CreateOptions{})
		if err != nil && !errors.IsAlreadyExists(err) {
			log.WithError(err).Error("create podgroup is failed")
			return err
		}
	}
	_, err := kubeclient.CoreV1().Pods(pod.Namespace).Create(context.TODO(), pod, metav1.CreateOptions{})
	return err
}

func buildAutoLearnPod(autoLearn *types.AutoLearn, revision *types.AutoLearnRevision) *corev1.Pod {
	autoLearnName := utils.GetAutoLearnMasterPodName(autoLearn.AutoLearnID, revision.RevisionID)

	brainppRuntimeStr := "false"
	if features.IsKubebrainRuntime() {
		brainppRuntimeStr = "true"
	}
	env := []corev1.EnvVar{
		{
			Name:  "OSS_ENDPOINT",
			Value: config.Profile.OssConfig.Endpoint,
		},
		{
			Name:  "AWS_ACCESS_KEY_ID",
			Value: config.Profile.OssConfig.AccessKey,
		},
		{
			Name:  "AWS_SECRET_ACCESS_KEY",
			Value: config.Profile.OssConfig.SecretKey,
		},
		{
			Name:  "AUTOLEARN_ID",
			Value: autoLearn.AutoLearnID,
		},
		{
			Name:  "AUTOLEARN_REVISION",
			Value: revision.RevisionID,
		},
		{
			Name:  "AUTOLEARN_TENANT",
			Value: autoLearn.TenantID,
		},
		{
			Name:  "AUTOLEARN_PROJECT",
			Value: autoLearn.ProjectID,
		},
		{
			Name:  aisConst.AISTenantEnvKey,
			Value: autoLearn.TenantID,
		},
		{
			Name:  aisConst.AISProjectEnvKey,
			Value: autoLearn.ProjectID,
		},
		{
			Name:  aisConst.AISResourceTypeEnvKey,
			Value: string(aisConst.ResourceTypeAutomaticLearning),
		},
		{
			Name:  aisConst.AISResourceIDEnvKey,
			Value: revision.RevisionID,
		},
		{
			Name:  aisConst.AISResourceNameEnvKey,
			Value: aisConst.B64EncodeK8SLabelValue(fmt.Sprintf("%s/%s", autoLearn.AutoLearnName, revision.RevisionName)),
		},
		{
			Name:  aisConst.AISUserIDEnvKey,
			Value: revision.CreatedBy.UserName,
		},
		{
			Name:  aisConst.AISUserNameEnvKey,
			Value: revision.CreatedBy.UserName,
		},
		{
			Name:  aisConst.AISProjectNameEnvKey,
			Value: autoLearn.ProjectID,
		},
		{
			Name:  aisConst.AISTenantNameEnvKey,
			Value: autoLearn.TenantID,
		},
		{
			Name:  "NORI_CONTROLLER_ADDR",
			Value: config.Profile.NoriServer.ControllerAddr,
		},
		{
			Name:  "NORI_LOCATER_ADDR",
			Value: config.Profile.NoriServer.LocateAddr,
		},
		{
			Name:  "APISERVER_BASE_URL",
			Value: "v1",
		},
		{
			Name:  "APISERVER_ADDR",
			Value: config.Profile.AisEndPoint.AutoLearn,
		},
		{
			Name:  "AJOB_ENDPOINT",
			Value: config.Profile.AisEndPoint.PublicServer,
		},
		{
			Name:  "BRAINPP_RUNTIME",
			Value: brainppRuntimeStr,
		},
		{
			Name:  "SNAPX_RESOURCE_MODE",
			Value: config.Profile.SnapxResouceMode,
		},
		{
			Name:  "NVIDIA_DRIVER_CAPABILITIES",
			Value: "compute,utility",
		},
		{
			Name:  "SNAPX_TASK_SCHEDULE_TIME",
			Value: fmt.Sprintf("%d", int(revision.ScheduleTimeLimit*60*60)),
		},
		{
			Name:  "AGENT_MODE",
			Value: "1",
		},
	}

	if config.Profile.DependentImage.SnapWorkerAgent != "" {
		env = append(env, corev1.EnvVar{
			Name:  "WORKER_AGENT_IMAGE",
			Value: config.Profile.DependentImage.SnapWorkerAgent,
		})
	}

	labels := map[string]string{
		config.LogsLabelKey:                            config.LogsLabelVal,
		publicTypes.AISAdmissionObjectSelectorLabelKey: publicTypes.AISAdmissionObjectSelectorLabelValue,
		types.AutolearnTypeKey:                         types.AutolearnTypeTaskValue, // 用于清理 snapx master pod
	}
	annotations := map[string]string{}

	commonLabels := &aisConst.CommonLabels{
		UserID:       revision.CreatedBy.UserName,
		UserName:     revision.CreatedBy.UserName,
		TenantID:     autoLearn.TenantID,
		TenantName:   autoLearn.TenantID,
		ProjectID:    autoLearn.ProjectID,
		ProjectName:  autoLearn.ProjectID,
		ResourceType: aisConst.ResourceTypeAutomaticLearning,
		ResourceID:   revision.RevisionID,
		ResourceName: fmt.Sprintf("%s/%s", autoLearn.AutoLearnName, revision.RevisionName),
	}
	commonLabels.Inject(annotations, labels)
	// 检查是否开启配额组
	if features.IsQuotaEnabled() {
		labels[aisConst.AISQuotaLabelKey] = aisConst.AISQuotaLabelValue
	}

	ndotsValue := "1"
	command := []string{types.WorkerInitCMD, "run", "--"}
	args := []string{types.InternalAgentCMD}
	sourceBinary := filepath.Join(types.InternalAgentImageBinaryDir, types.InternalAgentImageBinaryName)
	targetBinary := filepath.Join(types.SnapXImageInternalAgentBinaryDir, types.InternalAgentImageBinaryName)

	return &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Pod",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        autoLearnName,
			Namespace:   features.GetProjectWorkloadNamespace(autoLearn.ProjectID),
			Annotations: annotations,
			Labels:      labels,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:    autoLearnName,
				Image:   revision.SnapXImageURI,
				Command: command,
				Args:    args,
				Env:     env,
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("4"),
						corev1.ResourceMemory: resource.MustParse("32Gi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1"),
						corev1.ResourceMemory: resource.MustParse("16Gi"),
					},
				},
				VolumeMounts: []corev1.VolumeMount{
					{
						Name:      types.KubebrainToolsVolumeName,
						MountPath: types.KubebrainToolsMountDir,
					},
					{
						Name:      types.InitContainerShareVolumeName,
						MountPath: types.SnapXImageInternalAgentBinaryDir,
					},
				},
			}},
			InitContainers: []corev1.Container{{
				Name:    types.InitContainerName,
				Image:   config.Profile.DependentImage.InternalAgent,
				Command: []string{"cp", sourceBinary, targetBinary},
				VolumeMounts: []corev1.VolumeMount{
					{
						Name:      types.InitContainerShareVolumeName,
						MountPath: types.SnapXImageInternalAgentBinaryDir,
					},
				},
			}},
			Volumes: []corev1.Volume{
				{
					Name: types.KubebrainToolsVolumeName,
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: types.KubebrainToolsDir,
						},
					},
				},
				{
					Name: types.InitContainerShareVolumeName,
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
			},
			RestartPolicy: corev1.RestartPolicyNever,
			DNSConfig: &corev1.PodDNSConfig{
				Options: []corev1.PodDNSConfigOption{
					{
						Name:  "ndots",
						Value: &ndotsValue,
					},
				},
			},
		},
	}
}

func getInt32Ptr(i int32) *int32 {
	return &i
}
