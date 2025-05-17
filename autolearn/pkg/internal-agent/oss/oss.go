package oss

import (
	"bytes"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

const (
	OSS_ENDPOINT          = "OSS_ENDPOINT"
	AWS_ACCESS_KEY_ID     = "AWS_ACCESS_KEY_ID"
	AWS_SECRET_ACCESS_KEY = "AWS_SECRET_ACCESS_KEY"
)

type AbsoluteURI struct {
	uri string
}

func (oau *AbsoluteURI) GetBucket() string {
	u, _ := url.Parse(oau.uri)
	return u.Host
}

func (oau *AbsoluteURI) GetKey() string {
	u, _ := url.Parse(oau.uri)
	return strings.TrimLeft(u.Path, "/")
}

type S3Base struct {
	S3Conf *AwsConfig

	s3Client    *s3.S3
	s3ClientCtx aws.Context
}

func NewS3Base(s3Conf *AwsConfig) *S3Base {
	return &S3Base{
		S3Conf:      s3Conf,
		s3Client:    s3Client(s3Conf.Endpoint, s3Conf.AccessKeyID, s3Conf.AccessKeySecret),
		s3ClientCtx: aws.BackgroundContext(),
	}
}

func s3Client(endpoint, accessKey, secretKey string) *s3.S3 {
	config := &aws.Config{
		Endpoint:         &endpoint,
		Region:           aws.String("dummy"),
		S3ForcePathStyle: aws.Bool(true),
		Credentials:      credentials.NewStaticCredentials(accessKey, secretKey, ""),
	}

	sess := session.Must(session.NewSession())
	return s3.New(sess, config)
}

func (s3b *S3Base) Download(src string, dst string) error {
	// Create a downloader with the session and default options
	downloader := s3manager.NewDownloaderWithClient(s3b.s3Client)
	uri := AbsoluteURI{
		uri: src,
	}

	bucket := uri.GetBucket()
	key := uri.GetKey()

	log.Infof("[agent] src: %v", src)
	log.Infof("[agent] bucket: %v", bucket)
	log.Infof("[agent] key: %v", key)

	// Create a file to write the S3 Object contents to.
	dir := filepath.Dir(dst)
	err := os.MkdirAll(dir, os.ModePerm)

	if err != nil && !os.IsExist(err) {
		return err
	}

	writer, err := os.Create(dst)
	if err != nil {
		return err
	}

	// Write the contents of S3 Object to the file
	_, err = downloader.Download(writer, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})

	return err
}

func (s3b *S3Base) Upload(file string, dst string) (string, error) {
	uploader := s3manager.NewUploaderWithClient(s3b.s3Client)
	uri := AbsoluteURI{
		uri: dst,
	}
	bucket := uri.GetBucket()
	key := uri.GetKey()

	log.Infof("[agent] dst: %v", dst)
	log.Infof("[agent] bucket: %v", bucket)
	log.Infof("[agent] key: %v", key)

	data, err := os.ReadFile(file)
	if err != nil {
		log.WithError(err).Errorf("[agent] Read file %s error", file)
		return "", err
	}
	up, err := uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   bytes.NewReader(data),
	})
	if err != nil {
		log.WithError(err).Errorf("[agent] upload file %s error", file)
		return "", err
	}
	return up.Location, nil
}

func (s3b *S3Base) Head(src string) error {
	uri := AbsoluteURI{
		uri: src,
	}
	bucket := uri.GetBucket()
	key := uri.GetKey()

	log.Infof("[agent] bucket: %v", bucket)
	log.Infof("[agent] key: %v", key)

	headObjectInput := s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}
	if _, err := s3b.s3Client.HeadObject(&headObjectInput); err != nil {
		return err
	}
	return nil
}
