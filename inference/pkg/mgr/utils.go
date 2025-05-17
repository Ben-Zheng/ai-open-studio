package mgr

import (
	"encoding/base64"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/segmentio/ksuid"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/features"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/oss"
)

var (
	ErrInsufficientParam = errors.New("insufficient param")

	KFServingPodsKey    = "kfservingPods"
	KFServingPodsValue  = "true"
	KFServingTenantKey  = "tenant"
	KFServingProjectKey = "project"

	KserveWebhookLabelKeyIsvc        = "serving.kserve.io/inferenceservice"
	KserveWebhookLabelKeyComponent   = "component"
	KserveWebhookLabelValuePredictor = "predictor"
)

func getRequestParam(ctx *ginlib.GinContext, key string, optional bool) (string, error) {
	value, exist := ctx.GetRequestParam(key)
	if !exist {
		if !optional {
			return "", ErrInsufficientParam
		}
		return "", nil
	}
	return value, nil
}

func generateServiceID() string {
	return fmt.Sprintf("infer%s", ksuid.New().String()[:16])
}

func getIntReference(number int) *int {
	num := number
	return &num
}

// convertResourceLimits trans format to k8s pod resource limit format
// cpu: per cpu, gpu: per gpu, memory: GB
func convertResourceLimits(cpu int32, gpu int32, memory int32, gpuResourceName corev1.ResourceName) corev1.ResourceList {
	return corev1.ResourceList{
		"cpu":           resource.MustParse(strconv.Itoa(int(cpu))),
		"memory":        resource.MustParse(fmt.Sprintf("%dGi", memory)),
		gpuResourceName: resource.MustParse(strconv.Itoa(int(gpu))),
	}
}

func genServiceName(namespace, serviceName, instanceType string) string {
	return fmt.Sprintf("%s-%s-%s", namespace, serviceName, instanceType)
}

func formalizeURL(url string) string {
	if url != "" && !strings.HasPrefix(url, "http://") {
		return fmt.Sprintf("http://%s", url)
	}

	return url
}

func insideInt32Range(target, min, max int) bool {
	return target >= min && target <= max
}

func GetNamespace(projectName string) string {
	return features.GetProjectWorkloadNamespace(projectName)
}

func GetMockGinContext(tenant string) *ginlib.GinContext {
	gc := ginlib.NewMockGinContext()
	gc.SetTenantName(tenant)
	return gc
}

func CheckTaskNameValid(name string) bool {
	re := regexp.MustCompile(`^[\p{Han}\da-zA-Z\-_.]{1,64}$`)
	return len(re.FindAllString(name, -1)) > 0
}

func cleanProxyRequestHeader(r *http.Request) {
	r.Header = http.Header{}
}

func uploadImageToS3(c *oss.Client, bucket, key, base64Data string) error {
	// 解码Base64数据
	imageData, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		log.Error("Failed to decode base64 data")
		return err
	}

	if err := c.WriteObject(bucket, key, imageData); err != nil {
		log.Error("failed to upload image to oss")
		return err
	}

	return nil
}

func GetRandomString(n int) string {
	str := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	bArr := []byte(str)
	var result []byte
	for i := 0; i < n; i++ {
		result = append(result, bArr[rand.Intn(len(bArr))])
	}
	return string(result)
}
