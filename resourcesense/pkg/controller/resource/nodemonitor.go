package resource

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/oss"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/consts"
)

const (
	NodeDetailBucketName = "system-ais-node-megvii-face"
	NodeDetailURL        = "https://hh-d.brainpp.cn/kapis/node.brainpp.cn/v1alpha1/tenants/megvii/projects/megvii-face/nodes/details?tenant=megvii&project=megvii-face&page=1&pageSize=100&group=ais&cascade=true&sortBy=name%3Aasc&quotagroup=ais"
	PodDetailURL         = "https://hh-d.brainpp.cn/kapis/node.brainpp.cn/v1alpha1/node/%s/pods?page=1&pageSize=10000&sortBy=name:asc&resourcetype=worker&resourcetype=workspace&resourcetype=rjob&resourcetype=other"
)

func GetNodeDetailKey(t *time.Time) string {
	return fmt.Sprintf("%s/%s/%s/detail-%s-node", t.Format("2006"), t.Format("01"), t.Format("02"), t.Format("2006-01-02T15:04"))
}

func GetPodDetailKey(t *time.Time, nodeName string) string {
	return fmt.Sprintf("%s/%s/%s/detail-%s-pod/%s", t.Format("2006"), t.Format("01"), t.Format("02"), t.Format("2006-01-02T15:04"), nodeName)
}

func NodeMonitor(ctx context.Context) {
	if !consts.ConfigMap.MonitorEnabled {
		return
	}

	interval := 60
	t := time.NewTicker(time.Second * time.Duration(interval))
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Info("shutdown node monitor...")
			return
		case <-t.C:
			log.Warn("load node from kubebrain")
			nodeDetail, err := loadDetail(NodeDetailURL)
			if err != nil {
				log.WithError(err).Error("failed to loadNodeDetail")
				continue
			}

			tm := time.Now()

			if err := saveDetail(GetNodeDetailKey(&tm), nodeDetail); err != nil {
				log.WithError(err).Error("failed to saveNodeDetail")
				continue
			}

			var nds struct {
				Data *NodeDetails `json:"data"`
			}
			if err := json.Unmarshal(nodeDetail, &nds); err != nil {
				log.WithError(err).WithField("nodeDetail", string(nodeDetail)).Error("failed to Unmarshal nodeDetail")
				continue
			}

			for _, mc := range nds.Data.Machines {
				podDetail, err := loadDetail(fmt.Sprintf(PodDetailURL, mc.Metadata.Name))
				if err != nil {
					log.WithError(err).Error("failed to loadPodDetail")
					continue
				}

				if err := saveDetail(GetPodDetailKey(&tm, mc.Metadata.Name), podDetail); err != nil {
					log.WithError(err).Error("failed to savePodDetail")
					continue
				}
			}
		}
	}
}

func loadDetail(detailURL string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout*time.Second)
	defer cancel()

	req, err := http.NewRequest("GET", detailURL, nil)
	if err != nil {
		log.WithError(err).Error("failed to get node detail request")
		return nil, err
	}
	if consts.ConfigMap.KubebrainAccessToken == nil {
		log.Error("not exist KubebrainAccessToken")
		return nil, errors.New("not exist KubebrainAccessToken")
	}

	authToken := fmt.Sprintf("Basic %s", base64.StdEncoding.EncodeToString([]byte(consts.ConfigMap.KubebrainAccessToken.AccessKey+":"+consts.ConfigMap.KubebrainAccessToken.SecretKey)))
	req.Header.Set("authorization", authToken)

	resp, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		log.WithError(err).Error("get node detail failed")
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("node detail get request with status code: %d", resp.StatusCode)
		log.WithError(err).WithField("request", req).Error("request get node detail failed ")
		return nil, err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error("failed to read response body")
		return nil, err
	}

	return body, nil
}

func saveDetail(keyName string, detail []byte) error {
	o := oss.OSS{
		Session:    consts.OSSSession,
		BucketName: NodeDetailBucketName,
		KeyName:    keyName,
		Data:       detail,
	}

	return o.UploadBytes()
}

type NodeDetails struct {
	Machines []Machine `json:"machines"`
	Total    int       `json:"total"`
}

type Machine struct {
	Metadata *struct {
		Name string `json:"name"`
	} `json:"metadata"`
}
