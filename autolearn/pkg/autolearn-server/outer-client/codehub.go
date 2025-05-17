package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"

	authTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/auth/pkg/app/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/config"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/ctx/errors"
	codeTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/codehub/pkg/types"
)

func GetCodebaseRevision(tenantID, projectID, revisionID string) (*codeTypes.CodebaseRevision, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequest("GET",
		fmt.Sprintf("%s/v1/revisions/%s", config.Profile.AisEndPoint.Codehub, revisionID), nil)

	if err != nil {
		log.WithError(err).Error("failed to create evaluation request")
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(authTypes.AISProjectHeader, projectID)
	req.Header.Set(authTypes.AISTenantHeader, tenantID)
	resp, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		log.WithError(err).Errorf("failed to request codebase: %+v", req)
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.WithField("request", req).Errorf("response.status.code: %d", resp.StatusCode)
		return nil, errors.ErrorEvalHubService
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error("failed to read response body")
		return nil, err
	}

	revision := codeTypes.CodebaseRevision{}
	if err = json.Unmarshal(body, &revision); err != nil {
		log.WithError(err).Error("failed to unmarshal response body")
		return nil, err
	}

	return &revision, nil
}
