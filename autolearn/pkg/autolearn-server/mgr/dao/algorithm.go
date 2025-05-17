package dao

import (
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/ctx/errors"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/types"
	aisConst "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
)

func insertAlgorithm(d *DAO, autoLearnAlgorithm *types.AutoLearnAlgorithm) error {
	if _, err := d.Collections.AutoLearnAlgorithm().InsertOne(d.Context, autoLearnAlgorithm); err != nil {
		return err
	}
	return nil
}

func findAlgorithmByAutoLearnID(d *DAO, autoLearnID string) (map[string]*types.AutoLearnAlgorithm, error) {
	return findAlgorithmByAutoLearnIDs(d, []string{autoLearnID}, false)
}

func findAlgorithmByAutoLearnIDWithOrigianlInfo(d *DAO, autoLearnID string) (map[string]*types.AutoLearnAlgorithm, error) {
	return findAlgorithmByAutoLearnIDs(d, []string{autoLearnID}, true)
}

func findAlgorithmByAutoLearnIDs(d *DAO, autoLearnIDs []string, withOriginalInfo bool) (map[string]*types.AutoLearnAlgorithm, error) {
	projection := bson.M{"actualAlgorithms.originalInfo": 0, "originalAlgorithms.originalInfo": 0}
	if withOriginalInfo {
		projection = bson.M{}
	}
	opt := options.Find()
	opt.SetProjection(projection)
	cur, err := d.Collections.AutoLearnAlgorithm().Find(d.Context, bson.M{"autoLearnID": bson.M{"$in": autoLearnIDs}}, opt)
	if err != nil {
		return nil, err
	}

	res := make(map[string]*types.AutoLearnAlgorithm)
	for cur.Next(d.Context) {
		var temp types.AutoLearnAlgorithm
		if err := cur.Decode(&temp); err != nil {
			return nil, err
		}
		res[temp.RevisionID] = &temp
	}

	return res, nil
}

func findAlgorithmByRevisionID(d *DAO, revisionID string) (map[string]*types.AutoLearnAlgorithm, error) {
	return FindAlgorithmByRevisionIDs(d, []string{revisionID}, true)
}

func FindAlgorithmByRevisionIDs(d *DAO, revisionIDs []string, withOriginalInfo bool) (map[string]*types.AutoLearnAlgorithm, error) {
	projection := bson.M{"actualAlgorithms.originalInfo": 0, "originalAlgorithms.originalInfo": 0}
	if withOriginalInfo {
		projection = bson.M{}
	}
	opt := options.Find()
	opt.SetProjection(projection)
	cur, err := d.Collections.AutoLearnAlgorithm().Find(d.Context, bson.M{"revisionID": bson.M{"$in": revisionIDs}}, opt)
	if err != nil {
		return nil, err
	}

	res := make(map[string]*types.AutoLearnAlgorithm)
	for cur.Next(d.Context) {
		var temp types.AutoLearnAlgorithm
		if err := cur.Decode(&temp); err != nil {
			return nil, err
		}
		for _, alg := range temp.ActualAlgorithms {
			for _, hp := range alg.HyperParam {
				if va, ok := hp.Value.(bson.A); ok {
					for index := range va {
						if v, ok := va[index].(bson.D); ok {
							va[index] = v.Map()
						}
					}
				}
			}
		}
		res[temp.RevisionID] = &temp
	}

	return res, nil
}

func UpdateAlgorithmScore(d *DAO, revisionID,
	algorithmName,
	algorithmID string,
	score *types.Score,
	learnType aisConst.AppType,
	solutionType types.SolutionType) error {
	filter := bson.D{
		{Key: "revisionID", Value: revisionID},
		{Key: "actualAlgorithms.id", Value: algorithmID}}

	res, err := d.Collections.AutoLearnAlgorithm().UpdateOne(d.Context, filter, bson.M{"$push": bson.M{"actualAlgorithms.$.score": score}})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return errors.ErrorAutoLearnRvNotFound
	}

	return nil
}
