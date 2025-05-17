package v1

import (
	"net/http"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/mgr/hdlr"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
)

var Routes = func(mgr *hdlr.Mgr) []ginlib.Route {
	return []ginlib.Route{
		{
			Method:      http.MethodPost,
			Pattern:     "/v1/autolearns",
			HandlerFunc: mgr.CreateAutoLearn,
		},
		{
			Method:      http.MethodGet,
			Pattern:     "/v1/autolearns/:autolearnID",
			HandlerFunc: mgr.GetAutoLearn,
		},
		{
			Method:      http.MethodGet,
			Pattern:     "/v1/autolearns",
			HandlerFunc: mgr.ListAutoLearns,
		},
		{
			Method:      http.MethodGet,
			Pattern:     "/v1/revisions",
			HandlerFunc: mgr.ListRevisions,
		},
		{
			Method:      http.MethodGet,
			Pattern:     "/v1/last-autolearn",
			HandlerFunc: mgr.LastAutoLearn,
		},
		{
			Method:      http.MethodDelete,
			Pattern:     "/v1/autolearns/:autolearnID",
			HandlerFunc: mgr.DeleteAutoLearn,
		},
		{
			Method:      http.MethodPost,
			Pattern:     "/v1/autolearns/:autolearnID/revisions",
			HandlerFunc: mgr.CreateAutoLearnRevision,
		},
		{
			Method:      http.MethodGet,
			Pattern:     "/v1/autolearns/:autolearnID/revisions/:revisionID",
			HandlerFunc: mgr.GetAutoLearnRevision,
		},
		{
			Method:      http.MethodGet,
			Pattern:     "/v1/autolearns/:autolearnID/revisions/:revisionID/status",
			HandlerFunc: mgr.GetAutoLearnRevision,
		},
		{
			Method:      http.MethodGet,
			Pattern:     "/v1/autolearns/:autolearnID/revisions",
			HandlerFunc: mgr.ListAutoLearnRevisions,
		},
		{
			Method:      http.MethodDelete,
			Pattern:     "/v1/autolearns/:autolearnID/revisions/:revisionID",
			HandlerFunc: mgr.DeleteAutoLearnRevision,
		},
		{
			Method:      http.MethodPatch,
			Pattern:     "/v1/autolearns/:autolearnID/revisions/:revisionID",
			HandlerFunc: mgr.UpdateAutoLearnRevision,
		},
		{
			Method:      http.MethodPost,
			Pattern:     "/v1/autolearns/:autolearnID/revisions/:revisionID/rebuild",
			HandlerFunc: mgr.RebuildAutoLearnRevision,
		},
		{
			Method:      http.MethodPatch,
			Pattern:     "v1/autolearns/:autolearnID/revisions/:revisionID/callback",
			HandlerFunc: mgr.InternalAgentInfo,
		},
		{
			Method:      http.MethodGet,
			Pattern:     "v1/autolearns/:autolearnID/revisions/:revisionID/model.zip",
			HandlerFunc: mgr.ModelFileDownload,
		},
		{
			Method:      http.MethodPost,
			Pattern:     "/v1/recommend-sampler",
			HandlerFunc: mgr.RecommendSamplerBuild,
		},
		{
			Method:      http.MethodPut,
			Pattern:     "/v1/recommend-sampler",
			HandlerFunc: mgr.RecommendSamplerUpdate,
		},
		{
			Method:      http.MethodGet,
			Pattern:     "/v1/autolearns/:autolearnID/revisions/:revisionID/exportScore",
			HandlerFunc: mgr.ExportAlgorithmScore,
		},
		{
			Method:      http.MethodPost,
			Pattern:     "v1/autolearns/:autolearnID/revisions/:revisionID/evaluations",
			HandlerFunc: mgr.EvalJobRetry,
		},
		{
			Method:      http.MethodGet,
			Pattern:     "v1/autolearns-options",
			HandlerFunc: mgr.Options,
		},
		{
			Method:      http.MethodPost,
			Pattern:     "v1/dataset-attribute",
			HandlerFunc: mgr.GetDataSourceAttribute,
		},
		{
			Method:      http.MethodPost,
			Pattern:     "v1/datasets-attribute",
			HandlerFunc: mgr.GetDataSourcesAttribute,
		},
		{
			Method:      http.MethodPost,
			Pattern:     "/v1/autolearns/:autolearnID/revisions/:revisionID/optimization",
			HandlerFunc: mgr.AutoLearnRevisionOptimization,
		},
		{
			Method:      http.MethodPost,
			Pattern:     "/v1/comparisons",
			HandlerFunc: mgr.CreateComparison,
		},
		{
			Method:      http.MethodGet,
			Pattern:     "/v1/comparisons/:comparisonID/export",
			HandlerFunc: mgr.ExportComparison,
		},
		{
			Method:      http.MethodPost,
			Pattern:     "/v1/comparison-snapshots",
			HandlerFunc: mgr.SaveComparisonSnapshot,
		},
		{
			Method:      http.MethodGet,
			Pattern:     "/v1/comparison-snapshots",
			HandlerFunc: mgr.ListComparisonSnapshot,
		},
		{
			Method:      http.MethodGet,
			Pattern:     "/v1/comparison-snapshots/:comparisonSnapshotID",
			HandlerFunc: mgr.GetComparisonSnapshot,
		},
		{
			Method:      http.MethodDelete,
			Pattern:     "/v1/comparison-snapshots/:comparisonSnapshotID",
			HandlerFunc: mgr.DeleteComparisonSnapshot,
		},
		{ // 清理某个用户下的所有学习任务，运营端调用！！！
			Method:      http.MethodDelete,
			Pattern:     "/v1/internal/autolearns/release-by-user",
			HandlerFunc: mgr.ReleaseByUser,
		},
		{ // 生成AIT发版所需的模板信息
			Method:      http.MethodGet,
			Pattern:     "/v1/autolearns/:autolearnID/revisions/:revisionID/infoTemplate",
			HandlerFunc: mgr.ExportBasicInfo,
		},
	}
}
