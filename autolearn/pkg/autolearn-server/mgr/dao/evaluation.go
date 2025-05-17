package dao

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/ctx/errors"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/types"
	evalTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/evalhub/pkg/types"
)

func insertEvaluation(d *DAO, evaluation *types.AutoLearnEvaluation) error {
	if _, err := d.Collections.AutoLearnEvaluation().InsertOne(d.Context, evaluation); err != nil {
		return err
	}
	return nil
}

func findEvaluationByAutoLearnID(d *DAO, autoLearnID string) (map[string]*types.AutoLearnEvaluation, error) {
	return findEvaluationByAutoLearnIDs(d, []string{autoLearnID})
}

func findEvaluationByAutoLearnIDs(d *DAO, autoLearnIDs []string) (map[string]*types.AutoLearnEvaluation, error) {
	cur, err := d.Collections.AutoLearnEvaluation().Find(d.Context, bson.M{"autoLearnID": bson.M{"$in": autoLearnIDs}})
	if err != nil {
		return nil, err
	}

	res := make(map[string]*types.AutoLearnEvaluation)
	for cur.Next(d.Context) {
		var temp types.AutoLearnEvaluation
		if err := cur.Decode(&temp); err != nil {
			return nil, err
		}
		res[temp.RevisionID] = &temp
	}

	return res, nil
}

func findEvaluationByRevisionID(d *DAO, revisionID string) (map[string]*types.AutoLearnEvaluation, error) {
	return findEvaluationByRevisionIDs(d, []string{revisionID})
}

func findEvaluationByRevisionIDs(d *DAO, revisionIDs []string) (map[string]*types.AutoLearnEvaluation, error) {
	cur, err := d.Collections.AutoLearnEvaluation().Find(d.Context, bson.M{"revisionID": bson.M{"$in": revisionIDs}})
	if err != nil {
		return nil, err
	}

	res := make(map[string]*types.AutoLearnEvaluation)
	for cur.Next(d.Context) {
		var temp types.AutoLearnEvaluation
		if err := cur.Decode(&temp); err != nil {
			return nil, err
		}
		res[temp.RevisionID] = &temp
	}

	return res, nil
}

func findEvaluationByProjectID(d *DAO, projectID string) (map[string]*types.AutoLearnEvaluation, error) {
	filter := bson.D{
		{Key: "projectID", Value: projectID},
	}

	ops := options.Find().SetProjection(bson.D{
		{Key: "revisionID", Value: 1},
		{Key: "evaluationDetails.iterationNumber", Value: 1},
		{Key: "evaluationDetails.modelFilePath", Value: 1},
		{Key: "evaluationDetails.isDeleted", Value: 1},
	})

	cur, err := d.Collections.AutoLearnEvaluation().Find(context.Background(), filter, ops)
	if err != nil {
		return nil, err
	}

	res := make(map[string]*types.AutoLearnEvaluation)
	for cur.Next(d.Context) {
		var temp types.AutoLearnEvaluation
		if err := cur.Decode(&temp); err != nil {
			return nil, err
		}
		res[temp.RevisionID] = &temp
	}

	return res, nil
}

func AddEvaluationDetail(d *DAO, revisionID string, evaluationDetail *types.EvaluationDetail) error {
	filter := bson.D{
		{Key: "revisionID", Value: revisionID},
	}

	res, err := d.Collections.AutoLearnEvaluation().UpdateOne(d.Context, filter,
		bson.M{"$push": bson.M{"evaluationDetails": evaluationDetail}})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return errors.ErrorAutoLearnRvNotFound
	}

	return nil
}

func AddEvaluationDetailJob(d *DAO, revisionID string, iterationNumber int32, detailEval *types.EvaluationDetail) error {
	return updateEvaluationDetailJobCore(d, revisionID, iterationNumber, map[string]any{
		filedEvaluationID:       detailEval.EvaluationID,
		filedEvaluationName:     detailEval.EvaluationName,
		filedEvaluationJobID:    detailEval.EvaluationJobID,
		filedEvaluationJobName:  detailEval.EvaluationRJobName,
		filedEvaluationJobState: detailEval.EvaluationJobState,
		filedEvaluationJobs:     detailEval.EvaluationJobs,
		filedEvalDetSdsFilePath: detailEval.SdsFilePath,
		filedAutoLearnRvReason:  detailEval.Reason,
	})
}

func UpdateEvaluationDetailModelPath(d *DAO, revisionID string, iterationNumber int32, modelFilePath string) error {
	return updateEvaluationDetailJobCore(d, revisionID, iterationNumber, map[string]any{filedEvalDetModelPath: modelFilePath})
}

func UpdateEvaluationDetailStateAndReason(d *DAO,
	revisionID,
	reason string,
	state evalTypes.EvalJobResultState,
	iterationNumber int32,
	jobs ...*types.EvaluationJobDetail) error {
	update := map[string]any{filedEvaluationJobState: state, filedAutoLearnRvReason: reason}
	if len(jobs) > 0 {
		update[filedEvaluationJobs] = jobs
	}
	return updateEvaluationDetailJobCore(d, revisionID, iterationNumber, update)
}

func DeleteEvaluationDetail(d *DAO, revisionID string, iterationNumber int32) error {
	filter := bson.D{
		{Key: "revisionID", Value: revisionID},
		{Key: "evaluationDetails", Value: bson.M{
			"$elemMatch": bson.M{
				"iterationNumber":    iterationNumber,
				"evaluationJobState": bson.M{"$in": evalTypes.GetFailedJobStates()},
				"isDeleted":          bson.M{"$ne": true},
			}}},
	}

	res, err := d.Collections.AutoLearnEvaluation().UpdateOne(d.Context, filter, bson.M{"$set": bson.M{"evaluationDetails.$.isDeleted": true}})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return errors.ErrorAutoLearnRvNotFound
	}

	return nil
}

func UpdateDetEvalMetricMaxScore(d *DAO, revisionID string, maxScore float64) error {
	return updateEvaluationCore(d, revisionID, map[string]any{"evalMetric.maxScore": maxScore})
}

func UpdateClfEvalMetricMaxScore(d *DAO, revisionID string, maxScore float64) error {
	return updateEvaluationCore(d, revisionID, map[string]any{"clfEvalMetric.maxScore": maxScore})
}

func updateEvaluationCore(d *DAO, revisionID string, updateInfo map[string]any) error {
	filter := bson.D{
		{Key: "revisionID", Value: revisionID},
	}

	set := bson.M{}
	for k, v := range updateInfo {
		set[k] = v
	}

	res, err := d.Collections.AutoLearnEvaluation().UpdateOne(d.Context, filter, bson.M{"$set": set})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return errors.ErrorAutoLearnRvNotFound
	}

	return nil
}

func updateEvaluationDetailJobCore(d *DAO, revisionID string, iterationNumber int32, updateInfo map[string]any) error {
	filter := bson.D{
		{Key: "revisionID", Value: revisionID},
		{Key: "evaluationDetails", Value: bson.M{"$elemMatch": bson.M{"iterationNumber": iterationNumber, "isDeleted": bson.M{"$ne": true}}}},
	}

	set := bson.M{}
	for k, v := range updateInfo {
		mgoKey := fmt.Sprintf("evaluationDetails.$.%s", k)
		set[mgoKey] = v
	}

	res, err := d.Collections.AutoLearnEvaluation().UpdateOne(d.Context, filter, bson.M{"$set": set})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return errors.ErrorAutoLearnRvNotFound
	}

	return nil
}

func GetEvaluationJobWithoutFinalState(d *DAO) ([]*types.AutoLearnEvaluation, error) {
	unfinishedStates := evalTypes.GetUnfinishedJobStates()
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"evaluationDetails.evaluationJobs.evaluationJobState": bson.M{"$in": unfinishedStates}}}},
		{{Key: "$project", Value: bson.M{
			"_id":                                  0,
			"evaluationDetails.evaluationName":     0,
			"evaluationDetails.evaluationRJobName": 0,
			"evaluationDetails.sdsFilePath":        0,
			"evaluationDetails.modelFilePath":      0,
			"evaluationDetails.reason":             0,
		}}},
		{{Key: "$project", Value: bson.M{
			"projectID":   1,
			"autoLearnID": 1,
			"revisionID":  1,
			"evaluationDetails": bson.M{
				"$filter": bson.M{
					"input": "$evaluationDetails",
					"as":    "eval",
					"cond":  bson.M{"$ne": bson.A{"$$eval.isDeleted", true}},
				},
			},
		}}},
		{{Key: "$project", Value: bson.M{
			"projectID":   1,
			"autoLearnID": 1,
			"revisionID":  1,
			"evaluationDetails": bson.M{
				"$filter": bson.M{
					"input": "$evaluationDetails",
					"as":    "eval",
					"cond":  bson.M{"$in": bson.A{"$$eval.evaluationJobState", unfinishedStates}},
				},
			},
		}}},
	}

	cur, err := d.Collections.AutoLearnEvaluation().Aggregate(d.Context, pipeline)
	if err != nil {
		return nil, err
	}

	var res []*types.AutoLearnEvaluation
	if err := cur.All(d.Context, &res); err != nil {
		return nil, err
	}

	return res, nil
}

func UpdateEvaluationMetric(d *DAO, revisionID string, updateMetric *types.EvalMetric) error {
	filter := bson.D{
		{Key: "revisionID", Value: revisionID},
	}

	res, err := d.Collections.AutoLearnEvaluation().UpdateOne(d.Context, filter, bson.M{"$set": bson.M{"evalMetric": updateMetric}})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return errors.ErrorAutoLearnRvNotFound
	}

	return nil
}

func UpdateF1EvaluationMetric(d *DAO, revisionID string, updateMetric *types.EvalMetric) error {
	filter := bson.D{
		{Key: "revisionID", Value: revisionID},
	}

	res, err := d.Collections.AutoLearnEvaluation().UpdateOne(d.Context, filter, bson.M{"$set": bson.M{"f1EvalMetric": updateMetric}})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return errors.ErrorAutoLearnRvNotFound
	}

	return nil
}
