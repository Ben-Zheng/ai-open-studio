package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson/primitive"

	authTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/auth/pkg/app/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/config"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/ctx/errors"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/datahub/pkg/datahub-apiserver/types/dataset"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/authlib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
)

const (
	dataHubURLPrefix = "api/v1"

	// 请求evalHub接口超时时间，单位s
	dataHubRequestTimeout = 120
)

func GetPairRevision(gc *ginlib.GinContext, pairID, revisionID, originLevel string) (*Pair, error) {
	_, err := primitive.ObjectIDFromHex(pairID)
	if err != nil {
		log.WithError(err).WithField("pairID", pairID).Error("invalid pairID")
	}

	_, err = primitive.ObjectIDFromHex(revisionID)
	if err != nil {
		log.WithError(err).WithField("revisionID", revisionID).Error("invalid revisionID")
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), dataHubRequestTimeout*time.Second)
	defer cancel()

	req, err := http.NewRequest("GET", fmt.Sprintf("%s/%s/pairs/%s", config.Profile.AisEndPoint.DatahubAPIServer, dataHubURLPrefix, pairID), nil)
	if err != nil {
		log.WithError(err).Error("failed to create dataset pair request")
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(authlib.KBHeaderAuthorization, authlib.GenerateBasicAuthToken(config.Profile.AisAccessToken.AccessKey, config.Profile.AisAccessToken.SecretKey))
	req.Header.Set(authTypes.AISProjectHeader, gc.GetAuthProjectID())
	req.Header.Set(authTypes.AISTenantHeader, gc.GetAuthTenantID())
	req.Header.Set(authlib.HeaderUserID, gc.GetUserID())
	req.Header.Set(authlib.HeaderUserName, gc.GetUserName())

	resp, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		log.WithError(err).Errorf("failed to request datahub pair: %+v", req)
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.WithField("request", req).Errorf("response.status.code: %d", resp.StatusCode)
		return nil, errors.ErrorDataHubService
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error("failed to read response body")
		return nil, err
	}

	respData := struct {
		Data *Pair `json:"data"`
	}{&Pair{}}
	if err := json.Unmarshal(body, &respData); err != nil {
		log.WithError(err).Error("unmarshal dataset pair response failed")
		return nil, err
	}

	for i := range respData.Data.Revisions {
		log.Debugf("respData.revision: %+v", respData.Data.Revisions[i])
		if respData.Data.Revisions[i].ID.Hex() == revisionID {
			respData.Data.Revisions = []*Revision{respData.Data.Revisions[i]}
			return respData.Data, nil
		}
	}

	return nil, fmt.Errorf("dataset pair-%s not found with revision-%s", pairID, revisionID)
}

// Pair 仅为编译通过，coding时请使用实际定义
type Pair struct {
	ID          primitive.ObjectID `json:"id" bson:"_id"`
	Tenant      string             `json:"tenant" bson:"tenant"`
	Project     string             `json:"project" bson:"project"`
	Name        string             `json:"name" bson:"name"`
	Tags        []string           `json:"tags" bson:"tags"`
	Description string             `json:"description" bson:"description"`
	Revisions   []*Revision        `json:"revisions" bson:"revisions"`
}

type Revision struct {
	ID              primitive.ObjectID   `json:"id" bson:"_id"`
	Name            string               `json:"name" bson:"name"`
	PairID          primitive.ObjectID   `json:"pairID" bson:"pairID"`
	TrainDatasetRef *DatasetRef          `json:"trainDatasetRef" bson:"trainDatasetRef"`
	ValDatasetRef   *DatasetRef          `json:"valDatasetRef" bson:"valDatasetRef"`
	PublishState    dataset.PublishState `json:"publishState" bson:"publishState"`
}

// DatasetRef 关联的最终实际会使用的数据集
type DatasetRef struct {
	DatasetID  primitive.ObjectID `bson:"datasetID" json:"datasetID"`
	RevisionID primitive.ObjectID `bson:"revisionID" json:"revisionID"`
	SDSURI     string             `bson:"sdsURI" json:"sdsURI"`
}
