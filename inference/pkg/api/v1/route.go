package v1

import (
	"net/http"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/inference/pkg/mgr"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/auditlib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	"go.megvii-inc.com/brain/brainpp/projects/kubebrain/pkg/auditdb"
)

var (
	Routes = func(mgr *mgr.Mgr) []ginlib.Route {
		audit := auditlib.NewWrapAuditInterface()
		return []ginlib.Route{
			{
				Method:      http.MethodGet,
				Pattern:     "/v1/inferences",
				HandlerFunc: mgr.ListInferenceServices,
			},
			{
				Method:      http.MethodPost,
				Pattern:     "/v1/inferences",
				HandlerFunc: audit.GinWrapAuditMiddleware(auditdb.InferenceService, auditdb.CreateOperation, mgr.CreateInferenceService),
			},
			{
				Method:      http.MethodGet,
				Pattern:     "/v1/inferences/:inferenceServiceID",
				HandlerFunc: mgr.GetInferenceService,
			},
			{
				Method:      http.MethodPatch,
				Pattern:     "/v1/inferences/:inferenceServiceID",
				HandlerFunc: audit.GinWrapAuditMiddleware(auditdb.InferenceService, auditdb.UpdateOperation, mgr.UpdateInferenceService),
			},
			{
				Method:      http.MethodDelete,
				Pattern:     "/v1/inferences/:inferenceServiceID",
				HandlerFunc: audit.GinWrapAuditMiddleware(auditdb.InferenceService, auditdb.DeleteOperation, mgr.DeleteInferenceService),
			},
			{
				Method:      http.MethodPut,
				Pattern:     "/v1/inferences/:inferenceServiceID/reason",
				HandlerFunc: mgr.UpdateInferenceServiceReason,
			},
			{
				Method:      http.MethodGet,
				Pattern:     "/v1/inferences/:inferenceServiceID/revisions",
				HandlerFunc: mgr.ListInferenceServiceVersions,
			},
			{
				Method:      http.MethodGet,
				Pattern:     "/v1/inferences/:inferenceServiceID/revisions/:revisionID",
				HandlerFunc: mgr.GetInferenceServiceVersion,
			},
			{
				Method:      http.MethodPost,
				Pattern:     "/v1/inferences/:inferenceServiceID/revisions",
				HandlerFunc: audit.GinWrapAuditMiddleware(auditdb.InferenceService, auditdb.UpdateOperation, mgr.CreateInferenceServiceRevision),
			},
			{
				Method:      http.MethodPatch,
				Pattern:     "/v1/inferences/:inferenceServiceID/revisions/:revisionID",
				HandlerFunc: audit.GinWrapAuditMiddleware(auditdb.InferenceService, auditdb.UpdateOperation, mgr.PatchInferenceServiceRevision),
			},
			{
				Methods:     []string{http.MethodGet, http.MethodPost, http.MethodOptions},
				Pattern:     "/v1/inferences/:inferenceServiceID/apis/:instanceID/*apiURL",
				HandlerFunc: mgr.ProxyInferenceAPICall,
			},
			{
				Method:      http.MethodGet,
				Pattern:     "/v1/inferences/:inferenceServiceID/api-logs",
				HandlerFunc: mgr.ListInferenceServiceAPILogs,
			},
			{
				Method:      http.MethodGet,
				Pattern:     "/healthz",
				HandlerFunc: mgr.Healthz,
			},
			{
				Method:      http.MethodPost,
				Pattern:     "/v1/inferences/:inferenceServiceID/revisions/:revisionID/action",
				HandlerFunc: audit.GinWrapAuditMiddleware(auditdb.InferenceService, auditdb.UpdateOperation, mgr.InferenceRevisionAction),
			},
			{
				Method:      http.MethodDelete,
				Pattern:     "/v1/internal/infereces/release-by-user",
				HandlerFunc: mgr.ReleaseByUser,
			},
			{
				Method:      http.MethodPost,
				Pattern:     "/v1/inferences/:inferenceServiceID/revisions/:revisionID/conversationRecords",
				HandlerFunc: mgr.CreateConversationRecord,
			},
			{
				Method:      http.MethodGet,
				Pattern:     "/v1/inferences/:inferenceServiceID/revisions/:revisionID/conversationRecords/:recordID",
				HandlerFunc: mgr.GetConversationRecord,
			},
			{
				Method:      http.MethodGet,
				Pattern:     "/v1/inferences/:inferenceServiceID/revisions/:revisionID/conversationRecords",
				HandlerFunc: mgr.ListConversationRecord,
			},
			{
				Method:      http.MethodPatch,
				Pattern:     "/v1/inferences/:inferenceServiceID/revisions/:revisionID/conversationRecords/:recordID",
				HandlerFunc: mgr.UpdateConversationRecord,
			},
			{
				Method:      http.MethodDelete,
				Pattern:     "/v1/inferences/:inferenceServiceID/revisions/:revisionID/conversationRecords/:recordID",
				HandlerFunc: mgr.DeleteConversationRecord,
			},
			{
				Method:      http.MethodPost,
				Pattern:     "/v1/inferences/:inferenceServiceID/revisions/:revisionID/conversationRecords/:recordID/v1/chat/completions",
				HandlerFunc: mgr.ChatCompletion,
			},
			{
				Method:      http.MethodGet,
				Pattern:     "/v1/inferences/infer-resource",
				HandlerFunc: mgr.InferenceDefaultResource,
			},
		}
	}
)
