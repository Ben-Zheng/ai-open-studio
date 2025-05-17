package oss

import (
	"os"

	"gopkg.in/yaml.v2"
)

type Config struct {
	APIServerAddr string    `json:"apiserverAddr" yaml:"apiserverAddr"`
	AccessName    string    `json:"accessName" yaml:"accessName"`
	AccessSecret  string    `json:"accessSecret" yaml:"accessSecret"`
	OSS           AwsConfig `json:"oss" yaml:"oss"`
}

type AwsConfig struct {
	Endpoint        string `json:"endpoint" yaml:"endpoint"`
	AccessKeyID     string `json:"accessKeyID" yaml:"accessKeyID"`
	AccessKeySecret string `json:"accessKeySecret" yaml:"accessKeySecret"`
	Bucket          string `json:"bucket" yaml:"bucket"`
}

func LoadCfgFromYAML(path string) (*Config, error) {
	yamlStr, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(yamlStr, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
