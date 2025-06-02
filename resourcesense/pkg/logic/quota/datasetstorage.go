package quota

import (
	"fmt"

	log "github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson/primitive"
	k8sresource "k8s.io/apimachinery/pkg/api/resource"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/consts"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/controller/quota"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/controller/quota/datasetstorage"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types/ctx/errors"
	quotaTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types/quota"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/utils"
)

func CheckAndCalculateChargedQuotaReq(req *quotaTypes.TenantChargedQuotaUpdateReq) bool {
	if req.ResourceName != types.KubeResourceDatasetStorage {
		log.Errorf("unsupport resource: %s", req.ResourceName)
		return false
	}
	if req.ActionType != quotaTypes.ActionTypeFree && req.ActionType != quotaTypes.ActionTypeCharge {
		log.Errorf("unsupport action type: %s", req.ActionType)
		return false
	}

	if len(req.DatasetStorageQuota) == 0 {
		log.Errorf("request quota is zero")
		return false
	}

	req.TotalDatasetStorageQuota = make(quotaTypes.Quota)
	for i := range req.DatasetStorageQuota {
		objectID, err := primitive.ObjectIDFromHex(req.DatasetStorageQuota[i].DatasourceID)
		if err != nil {
			log.WithError(err).Error("failed to ObjectIDFromHex")
			return false
		}
		if objectID.IsZero() {
			req.DatasetStorageQuota[i].DatasourceID = ""
		}

		quantity, err := k8sresource.ParseQuantity(req.DatasetStorageQuota[i].RequestQuotaQuantity)
		if err != nil {
			log.WithError(err).Error("failed to parseQuantity")
			return false
		}
		req.DatasetStorageQuota[i].RequestQuota = quotaTypes.Quota{
			types.KubeResourceDatasetStorage: quantity,
		}

		req.TotalDatasetStorageQuota.InplaceAdd(req.DatasetStorageQuota[i].RequestQuota)
	}

	return true
}

func UpdateTenantChargedQuota(gc *ginlib.GinContext, req quotaTypes.TenantChargedQuotaUpdateReq) error {
	tenantID := gc.GetAuthTenantID()
	userName := gc.GetUserName()
	logger := log.WithFields(log.Fields{"username": userName, "tenantID": tenantID})

	unlock, err := utils.EtcdLocker(consts.EtcdClient, fmt.Sprintf(quotaTypes.DatasetStorageLock, tenantID), quotaTypes.LockTimeout)
	if err != nil {
		logger.WithError(err).Error("failed to EtcdLocker")
		return errors.ErrInternal
	}
	defer unlock()

	tenantDetail, err := quota.GetTenantDetail(gc, tenantID)
	if err != nil {
		logger.WithError(err).Error("failed to GetTenantDetail")
		return errors.ErrInternal
	}
	userDetail, err := quota.GetUserDetail(ginlib.NewMockGinContext(), tenantID, userName)
	if err != nil {
		logger.WithError(err).Error("failed to GetUserDetail")
		return errors.ErrInternal
	}
	usedResource := quota.GetUsedResourceNames(nil, quota.DatasetStorageResource, nil)

	switch req.ActionType {
	case quotaTypes.ActionTypeCharge:
		logger.Infof("[Quota_dataset] before, request quota: %+v, charged quota: %+v", req.TotalDatasetStorageQuota, tenantDetail.ChargedQuota)
		tenantDetail.ChargedQuota.InplaceAdd(req.TotalDatasetStorageQuota)
		logger.Infof("[Quota_dataset] after, charged quota: %+v, total quota: %+v", tenantDetail.ChargedQuota, tenantDetail.TotalQuota)
		if err := quota.LessThanOrEqual(tenantDetail.ChargedQuota, tenantDetail.TotalQuota, usedResource); err != nil {
			logger.WithError(err).Errorf("dataset storage quota is insufficient, totalQuota: %+v, chargedQuota: %+v", tenantDetail.TotalQuota, tenantDetail.ChargedQuota)
			return errors.ErrInsufficientAvailableQuota
		}
		userDetail.ChargedQuota.InplaceAdd(req.TotalDatasetStorageQuota)
	case quotaTypes.ActionTypeFree:
		tenantDetail.ChargedQuota.InplaceSub(req.TotalDatasetStorageQuota)
		tenantDetail.ChargedQuota.NoNegative()
		userDetail.ChargedQuota.InplaceSub(req.TotalDatasetStorageQuota)
		tenantDetail.ChargedQuota.NoNegative()
		logger.Infof("[Quota_dataset] free, request quota: %+v, charged quota: %+v", req.TotalDatasetStorageQuota, tenantDetail.ChargedQuota)
	}

	if err := quota.UpdateChargedQuota(gc, tenantID, tenantDetail.ChargedQuota); err != nil {
		logger.WithError(err).Error("failed to UpdateChargedQuota")
		return errors.ErrInternal
	}
	if err := quota.UpdateUserChargedQuota(gc, tenantID, userName, userDetail.ChargedQuota); err != nil {
		logger.WithError(err).Error("failed to UpdateUserChargedQuota")
		return errors.ErrInternal
	}

	datasetstorage.GetControllerManager(nil).HandleEvent(gc, &req)

	return nil
}
