package client

import (
	"encoding/json"
	"fmt"

	log "github.com/sirupsen/logrus"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/rest"
)

type Client interface {
	GetAutoLearn(gc *ginlib.GinContext, autolearnID string) (*types.AutoLearn, error)
	GetRevision(gc *ginlib.GinContext, autolearnID, revisionID string) (*types.AutoLearnRevision, error)
}

type autoLearnClient struct {
	autolearnURI string
	client       *rest.Client
}

func NewClient(autolearnURI string) Client {
	return &autoLearnClient{
		autolearnURI: autolearnURI,
		client:       rest.NewClient(),
	}
}

func (c *autoLearnClient) GetAutoLearn(gc *ginlib.GinContext, autoLearnID string) (*types.AutoLearn, error) {
	log.Debugf("Request autolearn-apiserver with token: %s", gc.GetAuthToken())
	request, err := c.client.GetRequest(gc)
	if err != nil {
		log.WithError(err).Error("failed to wrap resty request")
		return nil, err
	}

	resp, err := request.Get(fmt.Sprintf("%s/v1/autolearns/%s", c.autolearnURI, autoLearnID))
	if err != nil {
		log.WithError(err).Error("send autolearn get request failed")
		return nil, err
	}
	if resp.StatusCode() != 200 {
		err := fmt.Errorf("autolearn get response with status code: %d", resp.StatusCode())
		log.WithError(err).Error()
		return nil, err
	}

	autolearn := &types.AutoLearn{}
	if err := json.Unmarshal(resp.Body(), autolearn); err != nil {
		log.WithError(err).Error("unmarshal autolearn get response failed")
		return nil, err
	}

	return autolearn, nil
}

func (c *autoLearnClient) GetRevision(gc *ginlib.GinContext, autoLearnID, revisionID string) (*types.AutoLearnRevision, error) {
	log.Debugf("Request autolearn-apiserver with token: %s", gc.GetAuthToken())
	request, err := c.client.GetRequest(gc)
	if err != nil {
		log.WithError(err).Error("failed to wrap resty request")
		return nil, err
	}

	resp, err := request.Get(fmt.Sprintf("%s/v1/autolearns/%s/revisions/%s", c.autolearnURI, autoLearnID, revisionID))
	if err != nil {
		log.WithError(err).Error("send revision get request failed")
		return nil, err
	}
	if resp.StatusCode() != 200 {
		err := fmt.Errorf("revision get response with status code: %d", resp.StatusCode())
		log.WithError(err).Error()
		return nil, err
	}

	revision := &types.AutoLearnRevision{}
	if err := json.Unmarshal(resp.Body(), revision); err != nil {
		log.WithError(err).Error("unmarshal revision get response failed")
		return nil, err
	}

	return revision, nil
}
