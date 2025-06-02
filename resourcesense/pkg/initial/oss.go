package initial

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/consts"
)

// InitOSS 创建 oss 客户端
func InitOSS() {
	s3Config := &aws.Config{
		Credentials:      credentials.NewStaticCredentials(consts.ConfigMap.OSSConfig.AccessKeyID, consts.ConfigMap.OSSConfig.AccessKeySecret, ""),
		Endpoint:         aws.String(consts.ConfigMap.OSSConfig.Endpoint),
		Region:           aws.String("us-east-1"),
		DisableSSL:       aws.Bool(true),
		S3ForcePathStyle: aws.Bool(true),
	}
	newSession, err := session.NewSession(s3Config)
	if err != nil {
		panic(err)
	}
	consts.OSSSession = newSession
}
