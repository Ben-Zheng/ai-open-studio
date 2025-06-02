package pkg

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	authTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/auth/pkg/app/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/authlib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	aisTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
	algoresourceType "go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types/algoresource"
	quotaTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types/quota"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types/resource"
)

type Client interface {
	GetClusterAPU() ([]*aisTypes.APUConfig, error)
	CreateAudit(ra *resource.Audit) error
	ListAlgoResource(tenantID string) (*algoresourceType.TenantAlgo, error)
	UpdateAlgoResource(tenantID string, ar *algoresourceType.UpdateRequest) error
	ListDistributedTenantProjectByID(algoType, recordID string) ([]*algoresourceType.TenantProjectAlgo, error)
	DeleteDistributedRecord(tenantID, projectID, algoType, recordID string) error
	DeleteDistributedTenantRecord(tenantID, algoType, recordID, userID, userName string) error
	UpdateTenantChargedQuota(gc *ginlib.GinContext, updateReq *quotaTypes.TenantChargedQuotaUpdateReq) error
}

type RSClient struct {
	endpoint string
}

type AuthParam struct {
	TenantID, ProjectID string
	AuthToken           string
}

func NewRSClient(endpoint string) Client {
	return &RSClient{
		endpoint: endpoint,
	}
}

const (
	PublicServiceAPICallTimeout = time.Second * 180
)

func (c *RSClient) GetClusterAPU() ([]*aisTypes.APUConfig, error) {
	client := http.Client{}

	// prepare params
	requestURL := fmt.Sprintf("%s/api/v1/quotas/cluster/apu", c.endpoint)

	// create request
	request, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}

	request.Header.Set("Content-Type", "application/json")
	ctx, cancel := context.WithTimeout(request.Context(), PublicServiceAPICallTimeout)
	defer cancel()

	request = request.WithContext(ctx)
	resp, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		err := fmt.Errorf("request failed with status code: %d", resp.StatusCode)
		log.WithError(err).Errorf("request failed with status code: %d", resp.StatusCode)
		return nil, err
	}

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var respData struct {
		Data []*aisTypes.APUConfig `json:"data"`
	}

	if err := json.Unmarshal(responseBody, &respData); err != nil {
		log.WithError(err).Error("unmarshal dataset get response failed")
		return nil, err
	}

	return respData.Data, nil
}

func (c *RSClient) CreateAudit(ra *resource.Audit) error {
	client := http.Client{}

	// prepare params
	requestURL := fmt.Sprintf("%s/api/v1/audits", c.endpoint)
	requestBody, err := json.Marshal(ra)
	if err != nil {
		return err
	}

	// create request
	request, err := http.NewRequest(http.MethodPost, requestURL, bytes.NewReader(requestBody))
	if err != nil {
		return err
	}

	request.Header.Set("Content-Type", "application/json")
	ctx, cancel := context.WithTimeout(request.Context(), PublicServiceAPICallTimeout)
	defer cancel()

	request = request.WithContext(ctx)
	resp, err := client.Do(request)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		err := fmt.Errorf("request failed with status code: %d", resp.StatusCode)
		log.WithError(err).Errorf("request failed with status code: %d", resp.StatusCode)
		return err
	}

	return nil
}

func (c *RSClient) ListAlgoResource(tenantID string) (*algoresourceType.TenantAlgo, error) {
	client := http.Client{}

	// prepare params
	requestURL := fmt.Sprintf("%s/api/v1/algoresource/tenants/%s", c.endpoint, tenantID)
	// create request
	request, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}

	request.Header.Set("Content-Type", "application/json")
	ctx, cancel := context.WithTimeout(request.Context(), PublicServiceAPICallTimeout)
	defer cancel()

	request = request.WithContext(ctx)
	resp, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		err := fmt.Errorf("request failed with status code: %d", resp.StatusCode)
		log.WithError(err).Errorf("request failed with status code: %d", resp.StatusCode)
		return nil, err
	}

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	respData := struct {
		Data *algoresourceType.TenantAlgo `json:"data"`
	}{
		&algoresourceType.TenantAlgo{},
	}

	if err := json.Unmarshal(responseBody, &respData); err != nil {
		log.WithError(err).Error("unmarshal dataset get response failed")
		return nil, err
	}

	return respData.Data, nil
}

func (c *RSClient) UpdateAlgoResource(tenantID string, ar *algoresourceType.UpdateRequest) error {
	client := http.Client{}

	// prepare params
	requestURL := fmt.Sprintf("%s/api/v1/algoresource/tenants/%s", c.endpoint, tenantID)
	requestBody, err := json.Marshal(ar)
	if err != nil {
		return err
	}

	// create request
	request, err := http.NewRequest(http.MethodPut, requestURL, bytes.NewReader(requestBody))
	if err != nil {
		return err
	}

	request.Header.Set("Content-Type", "application/json")

	ctx, cancel := context.WithTimeout(request.Context(), PublicServiceAPICallTimeout)
	defer cancel()

	request = request.WithContext(ctx)
	resp, err := client.Do(request)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		err := fmt.Errorf("request failed with status code: %d", resp.StatusCode)
		log.WithError(err).Errorf("request failed with status code: %d", resp.StatusCode)
		return err
	}

	return nil
}

func (c *RSClient) ListDistributedTenantProjectByID(algoType, recordID string) ([]*algoresourceType.TenantProjectAlgo, error) {
	client := http.Client{}

	// prepare params
	requestURL := fmt.Sprintf("%s/api/v1/algoresource/records/%s/%s", c.endpoint, algoType, recordID)
	// create request
	request, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}

	request.Header.Set("Content-Type", "application/json")
	ctx, cancel := context.WithTimeout(request.Context(), PublicServiceAPICallTimeout)
	defer cancel()

	request = request.WithContext(ctx)
	resp, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		err := fmt.Errorf("request failed with status code: %d", resp.StatusCode)
		log.WithError(err).Errorf("request failed with status code: %d", resp.StatusCode)
		return nil, err
	}

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var respData listDistributedResponse
	if err := json.Unmarshal(responseBody, &respData); err != nil {
		log.WithError(err).Error("unmarshal dataset get response failed")
		return nil, err
	}

	return respData.Data.Items, nil
}

type listDistributedResponse struct {
	Data struct {
		Items []*algoresourceType.TenantProjectAlgo `json:"items"`
	} `json:"data"`
}

func (c *RSClient) DeleteDistributedRecord(tenantID, projectID, algoType, recordID string) error {
	client := http.Client{}

	// prepare params
	requestURL := fmt.Sprintf("%s/api/v1/algoresource/records/%s/%s/%s/%s", c.endpoint, tenantID, projectID, algoType, recordID)
	// create request
	request, err := http.NewRequest(http.MethodDelete, requestURL, nil)
	if err != nil {
		return err
	}

	request.Header.Set("Content-Type", "application/json")
	ctx, cancel := context.WithTimeout(request.Context(), PublicServiceAPICallTimeout)
	defer cancel()

	request = request.WithContext(ctx)
	resp, err := client.Do(request)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		err := fmt.Errorf("request failed with status code: %d", resp.StatusCode)
		log.WithError(err).Errorf("request failed with status code: %d", resp.StatusCode)
		return err
	}
	return nil
}

func (c *RSClient) DeleteDistributedTenantRecord(tenantID, algoType, recordID, userID, userName string) error {
	client := http.Client{}
	// prepare params
	requestURL := fmt.Sprintf("%s/api/v1/algoresource/records/tenant/%s/%s/%s", c.endpoint, tenantID, algoType, recordID)
	// create request
	request, err := http.NewRequest(http.MethodDelete, requestURL, nil)
	if err != nil {
		return err
	}

	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(authlib.HeaderUserID, userID)
	request.Header.Set(authlib.HeaderUserName, userName)
	ctx, cancel := context.WithTimeout(request.Context(), PublicServiceAPICallTimeout)
	defer cancel()

	request = request.WithContext(ctx)
	resp, err := client.Do(request)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		err := fmt.Errorf("request failed with status code: %d", resp.StatusCode)
		log.WithError(err).Errorf("request failed with status code: %d", resp.StatusCode)
		return err
	}
	return nil
}

func (c *RSClient) UpdateTenantChargedQuota(gc *ginlib.GinContext, updateReq *quotaTypes.TenantChargedQuotaUpdateReq) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	requestURL := fmt.Sprintf("%s/api/v1/quotas/tenants/%s/chargeQuota", c.endpoint, gc.GetAuthTenantID())
	requestBody, err := json.Marshal(updateReq)
	if err != nil {
		return err
	}

	request, err := http.NewRequest(http.MethodPut, requestURL, bytes.NewReader(requestBody))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(authTypes.AISProjectHeader, gc.GetAuthProjectID())
	request.Header.Set(authTypes.AISTenantHeader, gc.GetAuthTenantID())
	request.Header.Set(authlib.HeaderUserID, gc.GetUserID())
	request.Header.Set(authlib.HeaderUserName, gc.GetUserName())

	resp, err := http.DefaultClient.Do(request.WithContext(ctx))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	errResp := struct {
		Message string `json:"message"`
		SubCode string `json:"subCode"`
	}{}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error("Quota_dataset failed to read response body")
		return err
	}
	if resp.StatusCode != http.StatusNoContent {
		err := fmt.Errorf("request with status code: %d", resp.StatusCode)
		if resp.StatusCode != http.StatusBadRequest {
			log.WithField("Quota_dataset request body", updateReq).Error("failed to request resource chargeQuota")
			return err
		}

		if unmarshalErr := json.Unmarshal(body, &errResp); unmarshalErr != nil {
			log.WithError(unmarshalErr).Error("Quota_dataset failed to unmarshal")
			return err
		}

		log.Infof("Quota_dataset: %s", errResp.SubCode)
		return errors.New(errResp.SubCode)
	}

	return nil
}
