package thread

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	authTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/auth/pkg/app/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/utils"
)

type InternalAutoLearnRevision struct {
	Token         string
	Project       string
	Tenant        string
	Addr          string
	AutolearnName string
	*types.AutoLearnRevision
}

func GetAutolearnRevision() (*InternalAutoLearnRevision, error) {
	apiserverAddr := os.Getenv(utils.AUTOLEARN_APISERVER_ADDR)
	if apiserverAddr == "" {
		log.Error("[agent] get APISERVER_ADDR failed")
	}

	autolearnID := os.Getenv(utils.AutolearnID)
	if autolearnID == "" {
		log.Error("[agent] get autolearnID failed")
	}

	revision := os.Getenv(utils.AutolearnRevision)
	if revision == "" {
		log.Error("[agent] get revision failed")
	}

	tenant := os.Getenv(utils.AutolearnTenant)
	if tenant == "" {
		log.Error("[agent] get tenant failed")
	}

	project := os.Getenv(utils.AutolearnProject)
	if project == "" {
		log.Error("[agent] get project failed")
	}

	reqHeader := utils.AisRequestHeader{
		Tenant:  tenant,
		Project: project,
	}

	serverAddr := apiserverAddr
	if !strings.HasPrefix(serverAddr, "http") {
		serverAddr = "http://" + serverAddr
	}

	url := fmt.Sprintf("%s/v1/autolearns/%s/revisions/%s", serverAddr, autolearnID, revision)
	req, _ := http.NewRequest("GET", url, nil)

	req.Header.Set(authTypes.AISTenantHeader, reqHeader.Tenant)
	req.Header.Set(authTypes.AISProjectHeader, reqHeader.Project)

	log.Infof("[agent] get autolearn revision from apiserver url: %s, reqHeader: %+v", url, req.Header)
	rsp, err := (&http.Client{
		Timeout: time.Second * 30,
	}).Do(req)
	if err != nil {
		log.WithError(err).Error("[agent] get autolearn revision from autolearn apiserver failed!")
		return nil, err
	}

	var autolearnRevision types.AutoLearnRevision
	body, err := io.ReadAll(rsp.Body)
	if err != nil {
		log.WithError(err).Error("[agent] failed to read response body")
		return nil, err
	}
	if err = json.Unmarshal(body, &autolearnRevision); err != nil {
		log.WithError(err).Error("[agent] failed to unmarshal response body")
		return nil, err
	}

	defer rsp.Body.Close()
	if rsp.StatusCode > 300 {
		log.Errorf("[agent] stateCode: %d, url is: %s, data is: %s", rsp.StatusCode, url, string(body))
		return nil, err
	}

	log.Info("[agent] get autolearn revision success")

	return &InternalAutoLearnRevision{
		Project:           project,
		Tenant:            tenant,
		Addr:              serverAddr,
		AutoLearnRevision: &autolearnRevision,
	}, nil
}

func ReportPodState(state types.ActionState, errorCode int32) error {
	apiserverAddr := os.Getenv(utils.AUTOLEARN_APISERVER_ADDR)
	if apiserverAddr == "" {
		log.Error("[agent] get APISERVER_ADDR failed")
	}

	autolearnID := os.Getenv(utils.AutolearnID)
	if autolearnID == "" {
		log.Error("[agent] get autolearnID failed")
	}

	revision := os.Getenv(utils.AutolearnRevision)
	if revision == "" {
		log.Error("[agent] get revision failed")
	}

	tenant := os.Getenv(utils.AutolearnTenant)
	if tenant == "" {
		log.Error("[agent] get tenant failed")
	}

	project := os.Getenv(utils.AutolearnProject)
	if project == "" {
		log.Error("[agent] get project failed")
	}

	reqHeader := utils.AisRequestHeader{
		Tenant:  tenant,
		Project: project,
	}

	serverAddr := apiserverAddr
	if !strings.HasPrefix(serverAddr, "http") {
		serverAddr = "http://" + serverAddr
	}

	bodyData := types.InternalAgentInfoReq{State: state, AverageRecall: -1}
	if errorCode != 0 {
		bodyData.ErrorCode = errorCode
	}

	bodyBytes, err := json.Marshal(bodyData)
	if err != nil {
		log.Error("failed failed to marshal creatBody")
		return err
	}

	url := fmt.Sprintf("%s/v1/autolearns/%s/revisions/%s/callback", serverAddr, autolearnID, revision)
	req, _ := http.NewRequest("PATCH", url, bytes.NewBuffer(bodyBytes))

	req.Header.Set(authTypes.AISTenantHeader, reqHeader.Tenant)
	req.Header.Set(authTypes.AISProjectHeader, reqHeader.Project)
	req.Header.Set("Content-Type", "application/json")

	log.Infof("[agent] get autolearn revision from apiserver url: %s, reqHeader: %+v", url, req.Header)
	rsp, err := (&http.Client{
		Timeout: time.Second * 30,
	}).Do(req)
	if err != nil {
		log.WithError(err).Error("[agent] report master pod running failed!")
		return err
	}

	defer rsp.Body.Close()
	if rsp.StatusCode >= 300 {
		return err
	}

	return nil
}
