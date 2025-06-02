package resource

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"

	authv1 "go.megvii-inc.com/brain/brainpp/projects/aiservice/auth/pkg/api/v1"

	authTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/auth/pkg/app/types"
	datasetDB "go.megvii-inc.com/brain/brainpp/projects/aiservice/datahub/pkg/datahub-apiserver/types/dataset"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/authlib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/consts"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types/resource"
)

const (
	requestTimeout = 120
)

func GetUserByID(id string) (*authTypes.User, error) {
	authClient := authv1.NewClientWithAuthorazation(consts.ConfigMap.AuthServer,
		consts.ConfigMap.MultiSiteConfig.CenterAK, consts.ConfigMap.MultiSiteConfig.CenterSK)
	user, err := authClient.GetUser(nil, id)
	if err != nil {
		return nil, err
	}
	return user, err
}

func getUserList(page, pageSize int) ([]*authTypes.User, error) {
	authClient := authv1.NewClientWithAuthorazation(consts.ConfigMap.AuthServer,
		consts.ConfigMap.MultiSiteConfig.CenterAK, consts.ConfigMap.MultiSiteConfig.CenterSK)
	_, users, err := authClient.ListUser(nil, "createdAt", "desc", "", "", page, pageSize)
	return users, err
}

func getTenantProject(tenantID string, page, pageSize int) ([]*authTypes.Project, error) {
	authClient := authv1.NewClientWithAuthorazation(consts.ConfigMap.AuthServer,
		consts.ConfigMap.MultiSiteConfig.CenterAK, consts.ConfigMap.MultiSiteConfig.CenterSK)
	projects, err := authClient.ListProjectByTenant(nil, tenantID)
	return projects, err
}

func getDatasets(userName, tenantID, projectID, resourceType string, page, pageSize int) ([]*datasetDB.Meta, error) {
	var response struct {
		Total int64             `json:"total"`
		Data  []*datasetDB.Meta `json:"data"`
	}
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout*time.Second)
	defer cancel()

	req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/v1/datasets?page=%d&pageSize=%d&sortBy=createdAt&order=desc&resourceType=%s&createdBy=%s", consts.ConfigMap.DatasetServer, page, pageSize, resourceType, userName), nil)
	if err != nil {
		log.WithError(err).Error("failed to get dataset list request")
		return nil, err
	}
	if consts.ConfigMap.AccessToken != nil {
		authToken := fmt.Sprintf("Basic %s", base64.StdEncoding.EncodeToString([]byte(consts.ConfigMap.AccessToken.AccessKey+":"+consts.ConfigMap.AccessToken.SecretKey)))
		req.Header.Set("authorization", authToken)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(authTypes.AISTenantHeader, tenantID)
	req.Header.Set(authlib.HeaderSourceLevel, resourceType)
	req.Header.Set(authlib.KBHeaderProject, projectID)

	resp, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		log.WithError(err).Error("creat pair failed")
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("dataset get request with status code: %d", resp.StatusCode)
		log.WithError(err).WithField("request", req).Error("request dataset failed ")
		return nil, err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error("failed to read response body")
		return nil, err
	}
	if err := json.Unmarshal(body, &response); err != nil {
		log.WithError(err).Error("unmarshal dataset get response failed")
		return nil, err
	}

	return response.Data, nil
}

func getOssBytes(datasetID, tenantID, projectID, resourceType string) (int64, error) {
	var response struct {
		Data datasetDB.Meta `json:"data"`
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout*time.Second)
	defer cancel()

	req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/v1/datasets/%s", consts.ConfigMap.DatasetServer, datasetID), nil)
	if err != nil {
		log.WithError(err).Error("failed to get datasets request")
		return 0, err
	}
	if consts.ConfigMap.AccessToken != nil {
		authToken := fmt.Sprintf("Basic %s", base64.StdEncoding.EncodeToString([]byte(consts.ConfigMap.AccessToken.AccessKey+":"+consts.ConfigMap.AccessToken.SecretKey)))
		req.Header.Set("authorization", authToken)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(authTypes.AISTenantHeader, tenantID)
	req.Header.Set(authlib.HeaderSourceLevel, resourceType)
	req.Header.Set(authlib.KBHeaderProject, projectID)

	resp, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		log.WithError(err).Error("creat pair failed")
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("dataset get request with status code: %d", resp.StatusCode)
		log.WithError(err).WithField("request", req).Error("request dataset failed ")
		return 0, err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error("failed to read response body")
		return 0, err
	}
	if err := json.Unmarshal(body, &response); err != nil {
		log.WithError(err).Error("unmarshal dataset get response failed")
		return 0, err
	}

	var size int64
	for _, dr := range response.Data.Revisions {
		size += dr.MetaStat.Size
	}

	return size, nil
}

func getNasSpeedBytes(sdsURI string) (int64, error) {
	var response struct {
		Data int64 `json:"data"`
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout*time.Second)
	defer cancel()

	req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/v1/public/nori/sds/speedup?sdsURI=%s", consts.ConfigMap.PublicServer, sdsURI), nil)
	if err != nil {
		log.WithError(err).Error("failed to create dataset pair request")
		return 0, err
	}
	if consts.ConfigMap.AccessToken != nil {
		authToken := fmt.Sprintf("Basic %s", base64.StdEncoding.EncodeToString([]byte(consts.ConfigMap.AccessToken.AccessKey+":"+consts.ConfigMap.AccessToken.SecretKey)))
		req.Header.Set("authorization", authToken)
	}

	resp, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		log.WithError(err).Error("creat pair failed")
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("dataset get request with status code: %d", resp.StatusCode)
		log.WithError(err).WithField("request", req).Error("request dataset failed ")
		return 0, err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error("failed to read response body")
		return 0, err
	}
	if err := json.Unmarshal(body, &response); err != nil {
		log.WithError(err).Error("unmarshal dataset get response failed")
		return 0, err
	}

	return response.Data, nil
}

func computeUserMem(userName, tenantID string) (float64, float64, error) {
	var userOssBytes, userNasSpeedBytes float64

	page := 1
	pageSize := 100
	for {
		ps, err := getTenantProject(tenantID, page, pageSize)
		if err != nil {
			log.WithError(err).Error("failed to getDatasets")
			break
		}

		for i := range ps {
			userProjectOSSBytes, userProjectNASSpeedBytes, err := computeUserProjectMem(userName, tenantID, ps[i].ID, types.ProjectLevel)
			if err != nil {
				log.WithError(err).Error("failed to computeUserProjectMem")
			}

			userPublicOSSBytes, userPublicNASSpeedBytes, err := computeUserProjectMem(userName, tenantID, ps[i].ID, types.PublicLevel)
			if err != nil {
				log.WithError(err).Error("failed to computeUserProjectMem")
			}

			userOssBytes += userProjectOSSBytes + userPublicOSSBytes
			userNasSpeedBytes += userProjectNASSpeedBytes + userPublicNASSpeedBytes
		}

		if len(ps) < pageSize {
			break
		}
		page++
	}
	return userOssBytes, userNasSpeedBytes, nil
}

func computeUserProjectMem(userName, tenantID, projectID, resourceType string) (float64, float64, error) {
	var userOssBytes, userNasSpeedBytes float64

	page := 1
	pageSize := 100
	for {
		datasets, err := getDatasets(userName, tenantID, projectID, resourceType, page, pageSize)
		if err != nil {
			log.WithError(err).Error("failed to getDatasets")
			break
		}

		for _, ds := range datasets {
			log.WithField("userName", userName).WithField("tenantID", tenantID).WithField("projectID", projectID).Debugln(ds)
			ossBytes, err := getOssBytes(ds.ID.Hex(), tenantID, projectID, resourceType)
			if err != nil {
				log.WithError(err).Error("failed to getOssBytes")
			}
			userOssBytes += float64(ossBytes)

			for _, rev := range ds.Revisions {
				nasSpeedBytes, err := getNasSpeedBytes(rev.SdsURI)
				if err != nil {
					log.WithError(err).Error("failed to getNasSpeedBytes")
				}

				userNasSpeedBytes += float64(nasSpeedBytes)
			}
		}

		if len(datasets) < pageSize {
			break
		}
		page++
	}

	return userOssBytes, userNasSpeedBytes, nil
}

func loadUserInfo(resourceMap map[string]map[string]*markResource, isLoadStorage bool) error {
	page := 1
	pageSize := 100

	for {
		users, err := getUserList(page, pageSize)
		if err != nil {
			log.WithError(err).Error("failed to getUserList")
			return err
		}

		for i := range users {
			for j := range users[i].Tenants {
				aisUserID := users[i].ID
				aisUser := users[i].Name
				aisTenantID := users[i].Tenants[j].ID
				aisTenant := users[i].Tenants[j].Name
				createdAt := users[i].Tenants[j].CreatedAt
				aisTenantType := consts.TenantTypeOther
				if users[i].Tenants[j].TenantType == consts.TenantTypeWorkspace {
					aisTenantType = consts.TenantTypeWorkspace
				}

				if createdAt == 0 {
					createdAt = time.Now().Unix()
				}

				if _, ok := resourceMap[aisUserID]; !ok {
					tempMap := make(map[string]*markResource)
					resourceMap[aisUserID] = tempMap
				}
				if _, ok := resourceMap[aisUserID][aisTenantID]; !ok {
					mr := markResource{
						Resource: resource.Resource{
							UserID:   aisUserID,
							TenantID: aisTenantID,
						},
					}
					resourceMap[aisUserID][aisTenantID] = &mr
				}

				resourceMap[aisUserID][aisTenantID].CreatedAt = createdAt
				resourceMap[aisUserID][aisTenantID].TenantType = aisTenantType
				resourceMap[aisUserID][aisTenantID].UserName = helpDefaultEmpty(aisUser, false)
				resourceMap[aisUserID][aisTenantID].TenantName = helpDefaultEmpty(aisTenant, false)

				if isLoadStorage {
					userOssBytes, userNasSpeedBytes, err := computeUserMem(users[i].Name, users[i].Tenants[j].ID)
					if err != nil {
						log.WithError(err).Error("failed to computeUserMem")
						continue
					}

					resourceMap[aisUserID][aisTenantID].UsedQuota.HDD = userOssBytes
					resourceMap[aisUserID][aisTenantID].UsedQuota.SSD = userNasSpeedBytes

					if err := updateResourceStorage(aisUserID, aisTenantID, userOssBytes, userNasSpeedBytes); err != nil {
						log.WithError(err).Error("failed to updateResourceStorage")
					}
				}
			}
		}

		if len(users) < pageSize {
			break
		}
		page++
	}

	return nil
}

func StorageSync() {
	interval := 10
	t := time.NewTicker(time.Minute * time.Duration(interval))
	for {
		<-t.C
		log.Info("start sync StorageSync")

		resourceMap := make(map[string]map[string]*markResource)
		if err := loadUserInfo(resourceMap, true); err != nil {
			log.WithError(err).Error("failed to loadStorage")
			continue
		}
	}
}

func updateResourceStorage(aisUserID, aisTenantID string, userOssBytes, userNasSpeedBytes float64) error {
	gc := ginlib.NewMockGinContext()
	re, err := GetResource(gc, aisUserID, aisTenantID)
	if err != nil {
		log.WithError(err).Error("failed to GetResource")
		return err
	}

	re.UsedQuota.HDD = userOssBytes
	re.UsedQuota.SSD = userNasSpeedBytes
	if err := UpdateResourceByUUID(gc, re); err != nil {
		log.WithError(err).Error("failed to UpdateResource")
		return err
	}

	return nil
}
