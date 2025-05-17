package utils

import (
	"fmt"
	"math/rand"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/config"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/ctx/errors"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/features"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/oss"
)

func GetRecommendDirectory(random string) string {
	return fmt.Sprintf(config.RecommendWorkSpace, random)
}

func GenerateNameForOtherService(autoLearnName, revisionName, revisionID string) string {
	arr := []rune(autoLearnName + "-" + revisionName)
	if len(arr) <= config.InterceptedLength {
		return string(arr) + "-" + revisionID
	}
	return string(arr[:48]) + "-" + revisionID
}

func GenerateNameForOpRevision(originRvName string) string {
	newRvNameArr := []rune(config.OptimizationRvNamePrefix + originRvName + "-" + RandStringRunesWithLower(4))
	// 截取长度，提高辨识度
	if len(newRvNameArr) <= config.InterceptedLength {
		return string(newRvNameArr)
	}
	rvNameArr := []rune(originRvName)
	return config.OptimizationRvNamePrefix + string(rvNameArr[len(rvNameArr)-4:]) + "-" + RandStringRunesWithLower(4)
}

func GetAutoLearnMasterPodName(autolearnID, revisionID string) string {
	return config.AutoLearnPrefix + "-" + autolearnID + "-" + revisionID
}

// CheckOssPathExist 校验oss地址格式和key是否存在
func CheckOssPathExist(client *oss.Client, ossPath string) error {
	bucket, key, err := oss.ParseOSSPath(ossPath)
	if err != nil {
		return errors.ErrorInvalidS3URI
	}

	if valid, _ := client.CheckKeyExists(bucket, key); !valid {
		return errors.ErrorObjectNotFound
	}

	return nil
}

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
var lowerLetters = []rune("abcdefghijklmnopqrstuvwxyz")

func RandStringRunesWithLower(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(lowerLetters))]
	}
	return string(b)
}

func GetNamespace(projectName string) string {
	return features.GetProjectWorkloadNamespace(projectName)
}
