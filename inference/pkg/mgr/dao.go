package mgr

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/inference/pkg/types"
	inferenceErr "go.megvii-inc.com/brain/brainpp/projects/aiservice/inference/pkg/utils/ctx/errors"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/mgolib"
	aisTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
)

const (
	CollInference          = "inference"
	CollInferenceLog       = "inference_log"
	CollConversationRecord = "conversation_record"
)

const (
	InferenceServiceParamID         = "inferenceServiceID"
	InferenceServiceParamInstanceID = "instanceID"
	InferenceServiceParamAPIURL     = "apiURL"
	InferenceServiceParamRevisionID = "revisionID"
	InferenceServiceParamRecordID   = "recordID"

	InferenceServiceFieldID                 = "ID"
	InferenceServiceFieldName               = "name"
	InferenceServiceFieldProjectID          = "projectID"
	InferenceServiceFieldRevisions          = "revisions"
	InferenceServiceFieldDescription        = "description"
	InferenceServiceFieldTags               = "tags"
	InferenceServiceFieldUpdatedAt          = "updatedAt"
	InferenceServiceFieldUpdatedBy          = "updatedBy"
	InferenceServiceFieldInstancePods       = "instancePods"
	InferenceServiceFieldRevisionName       = "revisions.revision"
	InferenceServiceFieldServiceURI         = "serviceURI"
	InferenceServiceFieldServiceInternalURI = "serviceInternalURI"
	InferenceServiceFieldIsDeleted          = "isDeleted"
	InferenceServiceFieldRevisionCount      = "revisionCount"
	InferenceServiceRequestTotal            = "requestTotal"
	InferenceServiceRequestSuccessCount     = "requestSuccessCount"
	InferenceServiceFieldReady              = "revisions.%d.ready"
	InferenceServiceFieldStage              = "revisions.%d.stage"
	InferenceServiceFieldActive             = "revisions.%d.active"
	InferenceServiceFieldNotReadyReason     = "revisions.%d.serviceInstances.0.status.notReadyReason"
	InferenceServiceFieldFailedReason       = "revisions.%d.serviceInstances.0.status.failedReason"
	InferenceServiceAPILogFieldServiceID    = "inferenceServiceID"

	InferenceServiceInstanceUpdateTemplate = "revisions.%d.serviceInstances.0.%s"
	FieldInstanceImageURI                  = "meta.imageURI"
	FieldInstanceSnapModelVersion          = "meta.snapModelVersion"
	FieldInstanceSnapSolutionName          = "meta.snapSolutionName"
	FieldInstanceSnapModelProtocol         = "meta.snapModelProtocol"
	FieldInstanceReady                     = "status.ready"
	FieldInstanceNotReadyReason            = "status.notReadyReason"
	FieldInstanceInstanceURI               = "status.instanceURI"
	FieldInstanceUpdatedAt                 = "status.updatedAt"
)

// TODO: use lock for each project
var lock sync.Mutex

func (m *Mgr) getInferenceCollection(ctx *ginlib.GinContext) *mongo.Collection {
	return m.app.MgoClient.GetCollection(ctx, CollInference, func(connection *mongo.Collection) {
		connection.Indexes().CreateMany(context.Background(),
			[]mongo.IndexModel{
				{
					Keys: bson.M{
						InferenceServiceFieldProjectID: -1,
						InferenceServiceFieldName:      -1,
					},
					// nolint:staticcheck // MongoDB 从 4.2 开始放弃 SetBackground, 我们的 mongo 版本是 4.13
					Options: options.Index().SetBackground(true).SetUnique(true),
				},
				{
					Keys: bson.M{
						InferenceServiceFieldID: -1,
					},
					// nolint:staticcheck // MongoDB 从 4.2 开始放弃 SetBackground, 我们的 mongo 版本是 4.13
					Options: options.Index().SetBackground(true).SetUnique(true),
				},
			},
		)
	})
}

func getInferenceCollectionByTenant(mongoCli *mgolib.MgoClient, tenant string) *mongo.Collection {
	return mongoCli.GetCollectionByNameAndTenant(tenant, CollInference)
}

func (m *Mgr) getInferenceLogCollection(ctx *ginlib.GinContext) *mongo.Collection {
	return m.app.MgoClient.GetCollection(ctx, CollInferenceLog, func(connection *mongo.Collection) {
		expire := m.config.APILogRemainSeconds
		if expire <= 0 {
			expire = 30 * 24 * 3600
		}
		connection.Indexes().CreateMany(context.Background(),
			[]mongo.IndexModel{
				{
					Keys: bson.M{
						InferenceServiceFieldProjectID: -1,
						InferenceServiceFieldName:      -1,
					},
					// nolint:staticcheck // MongoDB 从 4.2 开始放弃 SetBackground, 我们的 mongo 版本是 4.13
					Options: options.Index().SetBackground(true),
				},
				{
					Keys: bson.M{
						"timestamp": 1,
					},
					// nolint:staticcheck // MongoDB 从 4.2 开始放弃 SetBackground, 我们的 mongo 版本是 4.13
					Options: options.Index().SetBackground(true).SetExpireAfterSeconds(expire),
				},
			},
		)
	})
}

func (m *Mgr) getConversationCollection(ctx *ginlib.GinContext) *mongo.Collection {
	return m.app.MgoClient.GetCollection(ctx, CollConversationRecord, func(collection *mongo.Collection) {
		collection.Indexes().CreateMany(context.Background(),
			[]mongo.IndexModel{
				{
					Keys: bson.M{
						"inferenceServiceID": -1,
						"isvcRevisionID":     -1,
					},
				},
			},
		)
	})
}

type ListQuery struct {
	Skip           int64
	Limit          int64
	Sort           bson.M
	ProjectID      string
	Names          []string
	AppTypes       []string
	CreatedByNames []string
	Tags           []string
	Descriptions   []string
	Approach       []string
	OnlyMe         bool
}

type ListResult struct {
	Data  []*types.InferenceService `json:"data" bson:"data"`
	Total int64                     `json:"total" bson:"total"`
}

// FindInferenceServices returns the InferenceServices with latest version
func FindInferenceServices(gc *ginlib.GinContext, col *mongo.Collection, query *ListQuery) (*ListResult, error) {
	if query == nil {
		query = &ListQuery{}
	}

	filter := bson.D{
		{Key: InferenceServiceFieldProjectID, Value: query.ProjectID},
		{Key: "$or", Value: bson.A{bson.M{InferenceServiceFieldIsDeleted: false}, bson.M{InferenceServiceFieldIsDeleted: bson.M{"$exists": false}}}},
	}
	if query.OnlyMe {
		filter = append(filter, bson.E{Key: "revisions.createdBy", Value: gc.GetUserName()})
	}
	appendOrRegexFilter(&filter, "createdBy", query.CreatedByNames)
	appendOrRegexFilter(&filter, "name", query.Names)
	appendOrRegexFilter(&filter, "tags", query.Tags)
	appendOrRegexFilter(&filter, "description", query.Descriptions)
	appendOrRegexFilter(&filter, "appType", query.AppTypes)
	var approachArray bson.A
	for i := range query.Approach {
		if query.Approach[i] == string(aisTypes.ApproachCV) {
			approachArray = append(approachArray, bson.M{"approach": bson.M{"$exists": false}}, bson.M{"approach": query.Approach[i]})
		} else {
			approachArray = append(approachArray, bson.M{"approach": query.Approach[i]})
		}
	}
	if len(approachArray) != 0 {
		filter = append(filter, bson.E{Key: "$or", Value: approachArray})
	}

	res := &ListResult{}
	var err error
	res.Total, err = col.CountDocuments(context.Background(), filter)
	if err != nil {
		return nil, err
	}
	var data []*types.InferenceService
	findOpt := findOptions(query)
	cursor, err := col.Find(context.Background(), filter, findOpt)
	if err != nil {
		return nil, err
	}
	if err := cursor.All(context.Background(), &data); err != nil {
		return nil, err
	}
	res.Data = data
	return res, nil
}

func appendOrRegexFilter(filter *bson.D, fieldName string, keyswords []string) {
	if len(keyswords) > 0 {
		cond := bson.A{}
		for _, name := range keyswords {
			cond = append(cond, bson.D{{Key: fieldName, Value: primitive.Regex{Pattern: name, Options: ""}}})
		}
		*filter = append(*filter, bson.E{Key: "$or", Value: cond})
	}
}

func ListInferenceServiceByUser(col *mongo.Collection, userName string) ([]*types.InferenceService, error) {
	cur, err := col.Find(context.TODO(), bson.M{"revisions": bson.M{"$elemMatch": bson.M{
		"createdBy": userName,
	}}}, findOptionWithLastRevision())
	if err != nil {
		return nil, err
	}

	var isvcs []*types.InferenceService
	if err := cur.All(context.TODO(), &isvcs); err != nil {
		return nil, err
	}
	return isvcs, nil
}

func FindInferenceServiceKANotZero(col *mongo.Collection) ([]*types.InferenceService, error) {
	cur, err := col.Find(context.TODO(), bson.M{"revisions": bson.M{"$elemMatch": bson.M{
		"serviceInstances.spec.keepAliveDuration": bson.M{"$gt": 0},
		"stage":  types.InferenceServiceRevisionStageServing,
		"active": true,
	}}}, findOptionWithLastRevision())
	if err != nil {
		return nil, err
	}

	var isvcs []*types.InferenceService
	if err := cur.All(context.TODO(), &isvcs); err != nil {
		return nil, err
	}

	return isvcs, nil
}

// FindInferenceRevisions returns all the history revisions of given service
func FindInferenceRevisions(col *mongo.Collection, projectID, serviceID string, pagination *ginlib.Pagination) (*types.InferenceService, error) {
	filter := serviceBaseFilter(projectID, serviceID)
	res := &types.InferenceService{}
	if err := col.FindOne(context.Background(), filter).Decode(res); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, inferenceErr.ErrDatabaseNotFound
		}
		return nil, err
	}

	pipeline := []bson.M{
		{"$match": filter},
		{"$unwind": "$revisions"},
		{"$sort": bson.M{"revisions.createdAt": -1}},
		{"$skip": pagination.Skip},
		{"$limit": pagination.Limit},
		{"$group": bson.M{"_id": nil, "revisions": bson.M{"$push": "$revisions"}}},
	}
	cur, err := col.Aggregate(context.TODO(), pipeline)
	if err != nil {
		return nil, err
	}

	var revisions []*types.InferenceService
	if err := cur.All(context.TODO(), &revisions); err != nil {
		return nil, err
	}
	res.Revisions = revisions[0].Revisions

	return res, nil
}

// FindInferenceService returns the target inference service with latest revision
func FindInferenceService(col *mongo.Collection, projectID, serviceID string) (*types.InferenceService, error) {
	res := &types.InferenceService{}
	if err := col.FindOne(context.Background(), serviceBaseFilter(projectID, serviceID), findOneOptionWithLastRevision()).Decode(res); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, inferenceErr.ErrDatabaseNotFound
		}
		return nil, err
	}

	return res, nil
}

// FindInferenceSpecificRevision inferservice中返回指定revision name
func FindInferenceSpecificRevision(col *mongo.Collection, projectID, serviceID, revisionName string) (*types.InferenceService, error) {
	filter := serviceBaseFilter(projectID, serviceID)
	filter[InferenceServiceFieldRevisionName] = revisionName
	res := &types.InferenceService{}
	if err := col.FindOne(context.Background(), filter).Decode(res); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, inferenceErr.ErrDatabaseNotFound
		}
		return nil, err
	}

	for i := range res.Revisions {
		if res.Revisions[i].Revision == revisionName {
			res.Revisions = []*types.InferenceServiceRevision{res.Revisions[i]}
			return res, nil
		}
	}

	return nil, inferenceErr.ErrDatabaseNotFound
}

// FindInferenceServiceWithRegexID returns the target inference service with the latest revision
func FindInferenceServiceWithRegexID(col *mongo.Collection, projectID, serviceID string) (*types.InferenceService, error) {
	res := &types.InferenceService{}
	if err := col.FindOne(context.Background(), serviceBaseFilter(projectID, serviceID), findOneOptionWithLastRevision()).Decode(res); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, inferenceErr.ErrDatabaseNotFound
		}
		return nil, err
	}
	return res, nil
}

func CreateInferenceService(col *mongo.Collection, service *types.InferenceService) error {
	if service == nil || len(service.Revisions) == 0 {
		return nil
	}
	lock.Lock()
	defer lock.Unlock()
	service.RevisionCount = 0
	service.Revisions[0].Revision = getNewRevision(service)
	service.Revisions[0].Active = true
	service.RevisionCount = 1
	if _, err := col.InsertOne(context.Background(), service); err != nil {
		return err
	}
	return nil
}

// ResetAndAddInferenceRevision any update will result in a new revision
func ResetAndAddInferenceRevision(gc *ginlib.GinContext, mc *mongo.Client, col *mongo.Collection, serviceID string, newRevision *types.InferenceServiceRevision) error {
	lock.Lock()
	defer lock.Unlock()

	filter := serviceBaseFilter(gc.GetAuthProjectID(), serviceID)
	oldService := &types.InferenceService{}
	if err := col.FindOne(context.Background(), filter, findOneOptionWithLastRevision()).Decode(oldService); err != nil {
		if err == mongo.ErrNoDocuments {
			return inferenceErr.ErrDatabaseNotFound
		}
		return err
	}
	if len(oldService.Revisions) == 0 {
		return errors.New("revisions is zero")
	}
	lastRevisionIndex := oldService.RevisionCount - 1

	// 1. reset state of the current active revision, update inference service basic info
	resetField := bson.M{"$set": bson.M{
		fmt.Sprintf(InferenceServiceFieldActive, lastRevisionIndex):                                      false,
		fmt.Sprintf(InferenceServiceFieldReady, lastRevisionIndex):                                       false,
		fmt.Sprintf(InferenceServiceInstanceUpdateTemplate, lastRevisionIndex, FieldInstanceReady):       false,
		fmt.Sprintf(InferenceServiceInstanceUpdateTemplate, lastRevisionIndex, FieldInstanceInstanceURI): "",
		fmt.Sprintf(InferenceServiceInstanceUpdateTemplate, lastRevisionIndex, FieldInstanceUpdatedAt):   time.Now().Unix(),
	}}

	// 2. update new revision info
	newRevision.Revision = getNewRevision(oldService)
	newRevision.Active = true
	if newRevision.ParentRevision == "" {
		newRevision.ParentRevision = oldService.Revisions[0].Revision
	}
	updateField := bson.M{
		"$set": bson.M{
			InferenceServiceFieldUpdatedBy:     newRevision.CreatedBy,
			InferenceServiceFieldUpdatedAt:     time.Now().Unix(),
			InferenceServiceFieldRevisionCount: oldService.RevisionCount + 1,
		},
		"$push": bson.M{
			InferenceServiceFieldRevisions: newRevision,
		},
	}

	// 3. update in transaction
	if err := updateLastRevisionAndPushNewRevision(mc, col, filter, resetField, updateField); err != nil {
		log.Error("failed to  updateLastRevisionAndPushNewRevision")
		return err
	}

	return nil
}

func updateLastRevisionAndPushNewRevision(client *mongo.Client, col *mongo.Collection, filter, resetField, updateField bson.M) error {
	return client.UseSession(context.TODO(), func(sctx mongo.SessionContext) error {
		if err := sctx.StartTransaction(options.Transaction().
			SetReadConcern(readconcern.Snapshot()).
			SetWriteConcern(writeconcern.New(writeconcern.WMajority()))); err != nil {
			return err
		}

		if _, err := col.UpdateOne(sctx, filter, resetField); err != nil {
			if e := sctx.AbortTransaction(sctx); e != nil {
				log.WithError(e).Error("AbortTransaction failed in update resetField")
			}
			log.Error("caught exception during transaction in UpdateALRevisionState, aborting...")
			return err
		}

		if _, err := col.UpdateOne(sctx, filter, updateField); err != nil {
			log.Error("failed to insertAlgorithmAndEvaluation")
			return err
		}

		return sctx.CommitTransaction(sctx)
	})
}

// UpdateInferenceServiceInfo 更新inferenceService基础信息
func UpdateInferenceServiceInfo(gc *ginlib.GinContext, col *mongo.Collection, serviceID string, updateInfo *types.PatchInferenceServiceReq) error {
	filter := serviceBaseFilter(gc.GetAuthProjectID(), serviceID)

	r, err := col.UpdateOne(context.Background(), filter, bson.M{
		"$set": bson.M{
			InferenceServiceFieldName:        updateInfo.Name,
			InferenceServiceFieldDescription: updateInfo.Description,
			InferenceServiceFieldTags:        updateInfo.Tags,
			InferenceServiceFieldUpdatedBy:   gc.GetUserName(),
			InferenceServiceFieldUpdatedAt:   time.Now().Unix(),
		},
	})
	if err != nil {
		return err
	}
	if r.MatchedCount == 0 {
		return inferenceErr.ErrDatabaseNotFound
	}

	return nil
}

func UpdateInferenceServiceReason(col *mongo.Collection, inferServiceID, projectID, reason string) error {
	filter := serviceBaseFilterWithoutDelete(projectID, inferServiceID)
	service := &types.InferenceService{}
	if err := col.FindOne(context.Background(), filter).Decode(service); err != nil {
		return err
	}

	update := bson.M{
		fmt.Sprintf(InferenceServiceFieldFailedReason, service.RevisionCount-1): reason,
	}

	if _, err := col.UpdateOne(context.Background(), filter,
		bson.M{
			"$set": update,
		}); err != nil {
		return err
	}
	return nil
}

func UpdateInferenceRevisionReadyAndStage(col *mongo.Collection, inferServiceID, projectID, reason string, ready bool, stage types.InferenceServiceRevisionStage) error {
	filter := serviceBaseFilterWithoutDelete(projectID, inferServiceID)
	service := &types.InferenceService{}
	if err := col.FindOne(context.Background(), filter).Decode(service); err != nil {
		return err
	}

	update := bson.M{
		// update latest revision's state
		fmt.Sprintf(InferenceServiceFieldReady, service.RevisionCount-1):          ready,
		fmt.Sprintf(InferenceServiceFieldStage, service.RevisionCount-1):          stage,
		fmt.Sprintf(InferenceServiceFieldNotReadyReason, service.RevisionCount-1): reason,
	}
	if _, err := col.UpdateOne(context.Background(), filter,
		bson.M{
			"$set": update,
		}); err != nil {
		return err
	}
	return nil
}

func UpdateInferenceServiceURL(col *mongo.Collection, inferServiceID, projectID, url, internalURI string) error {
	filter := serviceBaseFilter(projectID, inferServiceID)
	if _, err := col.UpdateOne(context.Background(), filter,
		bson.M{
			"$set": bson.M{
				InferenceServiceFieldServiceURI:         formalizeURL(url),
				InferenceServiceFieldServiceInternalURI: formalizeURL(internalURI),
			},
		}); err != nil {
		return err
	}
	return nil
}

func UpdateInferenceInstanceInfo(col *mongo.Collection, inferServiceID, projectID string, updateInfo map[string]interface{}) error {
	if len(updateInfo) == 0 {
		return nil
	}
	filter := serviceRegexIDFilterWithoutDelete(projectID, inferServiceID)
	lock.Lock()
	defer lock.Unlock()

	res := &types.InferenceService{}
	if err := col.FindOne(context.Background(), filter).Decode(res); err != nil {
		return err
	}

	update := bson.M{}
	for key := range updateInfo {
		update[fmt.Sprintf(InferenceServiceInstanceUpdateTemplate, res.RevisionCount-1, key)] = updateInfo[key]
	}

	if _, err := col.UpdateOne(context.Background(), filter, bson.M{"$set": update}); err != nil {
		return err
	}

	return nil
}

func UpdateSpecificRevisionInfo(gc *ginlib.GinContext, col *mongo.Collection, inferServiceID, revisionName string, updateInfo map[string]interface{}) error {
	filter := serviceBaseFilter(gc.GetAuthProjectID(), inferServiceID)
	filter[InferenceServiceFieldRevisionName] = revisionName
	update := bson.M{"updatedBy": gc.GetUserName(), "updatedAt": time.Now().Unix()}
	for key := range updateInfo {
		update["revisions.$."+key] = updateInfo[key]
	}

	ur, err := col.UpdateOne(context.TODO(), filter, bson.M{"$set": update})
	if err != nil {
		return err
	}
	if ur.MatchedCount == 0 {
		log.Infof("id, project, rvid: %s, %s, %s", inferServiceID, gc.GetAuthProjectID(), revisionName)
		return inferenceErr.ErrDatabaseNotFound
	}

	return nil
}

func UpdateLastRevisionToRestart(gc *ginlib.GinContext, col *mongo.Collection, inferServiceID, revisionName string) error {
	filter := serviceBaseFilter(gc.GetAuthProjectID(), inferServiceID)
	filter[InferenceServiceFieldRevisions] = bson.M{"$elemMatch": bson.M{
		"revision": revisionName,
		"stage":    types.InferenceServiceRevisionStageShutdown,
		"active":   true,
	}}
	update := bson.M{
		InferenceServiceFieldUpdatedBy: gc.GetUserName(),
		InferenceServiceFieldUpdatedAt: time.Now().Unix(),
		"revisions.$.stage":            types.InferenceServiceRevisionStageDeploying,
		"revisions.$.ready":            false,
		"revisions.$.createdAt":        time.Now().Unix(),
	}

	ur, err := col.UpdateOne(context.TODO(), filter, bson.M{"$set": update})
	if err != nil {
		return err
	}
	if ur.MatchedCount == 0 {
		return inferenceErr.ErrDatabaseNotFound
	}

	return nil
}

func DeleteInferenceService(gc *ginlib.GinContext, col *mongo.Collection, serviceID string) error {
	filter := serviceBaseFilter(gc.GetAuthProjectID(), serviceID)
	if _, err := col.UpdateOne(context.Background(), filter, bson.M{
		"$set": bson.M{
			InferenceServiceFieldIsDeleted: true,
			InferenceServiceFieldUpdatedBy: gc.GetUserName(),
			InferenceServiceFieldUpdatedAt: time.Now().Unix(),
		}}); err != nil {
		return err
	}

	return nil
}

func AddInstancePod(col *mongo.Collection, inferServiceID, projectID, podName, instanceType string, expire time.Duration) error {
	return col.Database().Client().UseSession(context.Background(), func(sctx mongo.SessionContext) error {
		filter := serviceBaseFilterWithoutDelete(projectID, inferServiceID)
		service := &types.InferenceService{}

		opt := options.FindOne().SetProjection(bson.M{InferenceServiceFieldInstancePods: 1})
		if err := col.FindOne(sctx, filter, opt).Decode(service); err != nil {
			return err
		}

		update := true
		for _, pod := range service.InstancePods {
			if pod.Name == podName {
				update = false
				break
			}
		}

		if update {
			// keep 7 days default
			if expire <= 0 {
				expire = 7 * 24 * time.Hour
			}

			now := time.Now()
			startTime := now.Add(-1 * expire).Unix()
			i := 0
			for i = 0; i < len(service.InstancePods); i++ {
				if service.InstancePods[i].CreatedAt > startTime {
					break
				}
			}
			instancePods := append(service.InstancePods[i:], &types.InstancePod{
				Name:      podName,
				CreatedAt: now.Unix(),
				Type:      instanceType,
			})
			if _, err := col.UpdateOne(sctx, filter,
				bson.M{
					"$set": bson.M{
						InferenceServiceFieldInstancePods: instancePods,
					}}); err != nil {
				return err
			}
		}
		return nil
	})
}

func AddInferenceServiceCallLog(col *mongo.Collection, logItem *types.InferenceRequestLog) error {
	if logItem == nil {
		return nil
	}

	if _, err := col.InsertOne(context.Background(), logItem); err != nil {
		log.WithError(err).Error("failed to insert call log")
		return err
	}

	return nil
}

type ListAPILogsResult struct {
	Data  []*types.InferenceRequestLog `json:"data" bson:"data"`
	Total int64                        `json:"total" bson:"total"`
}

// FindInferenceServiceAPILogs returns the InferenceService api call logs with service id
func FindInferenceServiceAPILogs(col *mongo.Collection, query *ListQuery, serviceID string) (*ListAPILogsResult, error) {
	if query == nil {
		query = &ListQuery{}
	}
	filter := bson.M{
		InferenceServiceAPILogFieldServiceID: serviceID,
	}

	res := &ListAPILogsResult{}
	var err error
	res.Total, err = col.CountDocuments(context.Background(), filter)
	if err != nil {
		return nil, err
	}
	var data []*types.InferenceRequestLog
	findOpt := findOptions(query)
	cursor, err := col.Find(context.Background(), filter, findOpt)
	if err != nil {
		return nil, err
	}
	if err := cursor.All(context.Background(), &data); err != nil {
		return nil, err
	}
	res.Data = data
	return res, nil
}

func InferenceRequestCountInc(col *mongo.Collection, serviceID string, success bool) error {
	update := bson.M{
		"$inc": bson.M{InferenceServiceRequestTotal: 1},
	}
	if success {
		update["$inc"].(bson.M)[InferenceServiceRequestSuccessCount] = 1
	}

	if _, err := col.UpdateOne(context.Background(),
		bson.M{InferenceServiceFieldID: serviceID},
		update); err != nil {
		log.WithError(err).Error("failed to inc call successful count")
		return err
	}

	return nil
}

// internal function
func findOptions(query *ListQuery) *options.FindOptions {
	opt := findOptionWithLastRevision().SetSort(query.Sort)
	if query.Skip > 0 {
		opt = opt.SetSkip(query.Skip)
	}
	if query.Limit > 0 {
		opt = opt.SetLimit(query.Limit)
	}

	return opt
}

// findOptionWithLastRevision returns the findoption with revision limit.
// only latest revision is returned
func findOptionWithLastRevision() *options.FindOptions {
	return options.Find().SetProjection(
		lastRevisionLimitBson(),
	)
}

func findOneOptionWithLastRevision() *options.FindOneOptions {
	return options.FindOne().SetProjection(
		lastRevisionLimitBson(),
	)
}

func findOneOptionWithPagedRevisions(skip, limit int64) *options.FindOneOptions {
	return options.FindOne().SetProjection(bson.M{
		InferenceServiceFieldRevisions: bson.M{
			"$slice": []any{skip, limit},
		},
	})
}

func lastRevisionLimitBson() bson.M {
	return bson.M{
		InferenceServiceFieldRevisions: bson.M{
			"$slice": -1,
		},
	}
}

func serviceBaseFilter(projectID, serviceID string) bson.M {
	filter := bson.M{InferenceServiceFieldID: serviceID}
	return baseFilterWrap(filter, projectID)
}

func serviceRegexIDFilter(projectID, serviceID string) bson.M {
	filter := bson.M{InferenceServiceFieldID: bson.M{"$regex": primitive.Regex{Pattern: fmt.Sprintf("^%s$", serviceID), Options: "im"}}}
	return baseFilterWrap(filter, projectID)
}

func serviceBaseFilterWithoutDelete(projectID, serviceID string) bson.M {
	filter := bson.M{InferenceServiceFieldID: serviceID, InferenceServiceFieldProjectID: projectID}
	return filter
}

func serviceRegexIDFilterWithoutDelete(projectID, serviceID string) bson.M {
	filter := bson.M{InferenceServiceFieldID: bson.M{"$regex": primitive.Regex{Pattern: fmt.Sprintf("^%s$", serviceID), Options: "im"}},
		InferenceServiceFieldProjectID: projectID,
	}
	return filter
}

func baseFilterWrap(filter bson.M, projectID string) bson.M {
	filter[InferenceServiceFieldProjectID] = projectID
	filter["$or"] = bson.A{bson.M{InferenceServiceFieldIsDeleted: false}, bson.M{InferenceServiceFieldIsDeleted: bson.M{"$exists": false}}}

	return filter
}

func getNewRevision(service *types.InferenceService) string {
	return fmt.Sprintf("v%d", service.RevisionCount+1)
}

func buildListQuery(ctx *ginlib.GinContext) (*ListQuery, error) {
	var (
		query *ListQuery
	)
	pagination := ctx.GetPagination()
	sort := ctx.GetSort()
	if sort == nil {
		sort = &ginlib.Sort{
			By:    "createdAt",
			Order: -1,
		}
	}
	if sort.By == "" {
		sort.By = "createdAt"
	}
	if sort.Order == 0 {
		sort.Order = -1
	}

	query = &ListQuery{
		Skip:  pagination.Skip,
		Limit: pagination.Limit,
		Sort: bson.M{
			sort.By: sort.Order,
		},
		ProjectID:      ctx.GetAuthProjectID(),
		AppTypes:       ctx.QueryArray("appType"),
		Names:          ctx.QueryArray("name"),
		CreatedByNames: ctx.QueryArray("createdBy"),
		Tags:           ctx.QueryArray("tag"),
		Descriptions:   ctx.QueryArray("description"),
		Approach:       ctx.QueryArray("approach"),
	}
	query.OnlyMe, _ = strconv.ParseBool(ctx.Query("onlyMe"))

	return query, nil
}

func IsDupErr(err error) bool {
	we, ok := err.(mongo.WriteException)
	if !ok {
		return false
	}
	for _, wce := range we.WriteErrors {
		if wce.Code == 11000 || wce.Code == 11001 || wce.Code == 12582 || wce.Code == 16460 && strings.Contains(wce.Message, "E11000 ") {
			return true
		}
	}
	return false
}

func InsertConversationRecord(col *mongo.Collection, conversation *types.ConversationRecord) error {
	_, err := col.InsertOne(context.TODO(), conversation)
	if err != nil {
		return err
	}
	return nil
}

func FindConversationRecord(col *mongo.Collection, recordID string) (*types.ConversationRecord, error) {
	id, err := primitive.ObjectIDFromHex(recordID)
	if err != nil {
		log.WithError(err).Error("failed to get objectID")
		return nil, inferenceErr.ErrInvalidParam
	}

	record := types.ConversationRecord{}
	if err := col.FindOne(context.TODO(), bson.M{"_id": id, "isDeleted": false}).Decode(&record); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, inferenceErr.ErrConversationRecordNotFound
		}
		return nil, err
	}

	return &record, nil
}

func FindConversationRecordByIsvcRvID(col *mongo.Collection, isvcID, isvcRvID string) ([]*types.ConversationRecord, error) {
	var results []*types.ConversationRecord
	cur, err := col.Find(context.TODO(), bson.M{
		"inferenceServiceID": isvcID,
		"isvcRevisionID":     isvcRvID,
		"isDeleted":          false,
	}, options.Find().SetSort(bson.M{"createdAt": -1}))
	if err != nil {
		return nil, err
	}
	if err := cur.All(context.TODO(), &results); err != nil {
		return nil, err
	}

	return results, nil
}

func UpdateConversationRecordName(col *mongo.Collection, recordID, name string) error {
	id, err := primitive.ObjectIDFromHex(recordID)
	if err != nil {
		log.WithError(err).Error("failed to get objectID")
		return inferenceErr.ErrInvalidParam
	}

	if _, err := col.UpdateOne(context.TODO(), bson.M{"_id": id, "isDeleted": false}, bson.M{"$set": bson.M{
		"name": name,
	}}); err != nil {
		return err
	}

	return nil
}

func DeleteConversationRecord(col *mongo.Collection, id primitive.ObjectID) error {
	if _, err := col.UpdateOne(context.TODO(), bson.M{"_id": id}, bson.M{"$set": bson.M{"isDeleted": true}}); err != nil {
		return err
	}

	return nil
}

func PushConversationIntoConversationRecord(col *mongo.Collection, id primitive.ObjectID, conversations []*types.Conversation, token int) error {
	if len(conversations) == 0 {
		return nil
	}

	_, err := col.UpdateOne(context.TODO(),
		bson.M{"_id": id, "isDeleted": false},
		bson.M{"$push": bson.M{"conversations": bson.M{"$each": conversations}}, "$inc": bson.M{"totalTokens": token}},
	)
	if err != nil {
		return err
	}

	return nil
}

func UpdateLogResponse(col *mongo.Collection, id primitive.ObjectID, response string) error {
	if response == "" {
		return nil
	}

	if _, err := col.UpdateOne(context.TODO(), bson.M{"_id": id}, bson.M{"$set": bson.M{"response": response}}); err != nil {
		return err
	}

	return nil
}

func DeleteConversationFromConversationRecord(col *mongo.Collection, conversationRecordID string, ids []primitive.ObjectID, token int) error {
	recordID, err := primitive.ObjectIDFromHex(conversationRecordID)
	if err != nil {
		return err
	}

	filter := bson.M{"_id": recordID}
	update := bson.M{"$pull": bson.M{"conversations": bson.M{"id": bson.M{"$in": ids}}}, "$inc": bson.M{"totalTokens": -token}}

	_, err = col.UpdateOne(context.TODO(), filter, update)
	if err != nil {
		log.WithError(err).Error("failed to updateOne")
	}

	return err
}
