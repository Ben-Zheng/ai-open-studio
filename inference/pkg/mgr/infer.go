package mgr

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/inference/pkg/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/inference/pkg/utils/ctx"
	inferenceErr "go.megvii-inc.com/brain/brainpp/projects/aiservice/inference/pkg/utils/ctx/errors"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/utils"
)

func (m *Mgr) ProxyInferenceAPICall(c *gin.Context) {
	gc := ginlib.NewGinContext(c)
	projectName := gc.GetAuthProjectID()

	serviceID, err := getRequestParam(gc, InferenceServiceParamID, false)
	if err != nil {
		log.WithError(err).Error("failed to get inference service id from request")
		ctx.Error(c, inferenceErr.ErrInvalidParam)
		return
	}
	instanceID, err := getRequestParam(gc, InferenceServiceParamInstanceID, false)
	if err != nil {
		log.WithError(err).Error("failed to get inference instanceID from request")
		ctx.Error(c, inferenceErr.ErrInvalidParam)
		return
	}
	if instanceID != InstanceIDService {
		log.Errorf("invalid instanceID: %s", instanceID)
		ctx.Error(c, inferenceErr.ErrInvalidParam)
		return
	}

	apiURL, err := getRequestParam(gc, InferenceServiceParamAPIURL, false)
	if err != nil {
		log.WithError(err).Error("failed to get inference apiURL from request")
		ctx.Error(c, inferenceErr.ErrInvalidParam)
		return
	}

	col := m.getInferenceCollection(gc)
	inferService, err := FindInferenceService(col, projectName, serviceID)
	if err != nil {
		log.WithError(err).Error("failed to get service")
		ctx.Error(c, err)
		return
	}
	if len(inferService.Revisions) == 0 {
		log.Error("nil revisions")
		ctx.Error(c, inferenceErr.ErrInternal)
		return
	}
	lastRevision := inferService.Revisions[len(inferService.Revisions)-1]
	if len(lastRevision.ServiceInstances) == 0 || lastRevision.ServiceInstances[0] == nil {
		log.Error("nil instances")
		ctx.Error(c, inferenceErr.ErrInternal)
		return
	}
	if inferService.ServiceInternalURI == "" {
		log.Error("empty service internal URI")
		ctx.Error(c, inferenceErr.ErrInternal)
		return
	}

	log.Infof("invoking inference url host: %s, path: %s", inferService.ServiceInternalURI, apiURL)

	colLog := m.getInferenceLogCollection(gc)
	addInferenceServiceLog := func(resp *http.Response) error {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		body := io.NopCloser(bytes.NewReader(respBody))
		resp.Body = body
		if resp.StatusCode != http.StatusOK {
			log.Errorf("failed to invoking inference url host: %s, path: %s, resp: %s", inferService.ServiceInternalURI, apiURL, string(respBody))
		}

		logItem := &types.InferenceRequestLog{
			ID:                 primitive.NewObjectID(),
			InferenceServiceID: inferService.ID,
			CreatedAt:          time.Now().Unix(),
			Source:             resp.Request.Header.Get("X-Forwarded-For"),
			Revision:           lastRevision.Revision,
			InstanceID:         resp.Header.Get(InferenceResponseHeaderInstanceIDName),
			APIURL:             apiURL,
			Response:           string(respBody),
			Status:             resp.StatusCode,
		}
		AddInferenceServiceCallLog(colLog, logItem)
		InferenceRequestCountInc(col, serviceID, resp.StatusCode == http.StatusOK)
		m.InferenceRequestCountMetricsInc(projectName, serviceID, logItem.InstanceID, resp.StatusCode)

		return nil
	}

	errorInferenceServiceLog := func(rw http.ResponseWriter, r *http.Request, err error) {
		logItem := &types.InferenceRequestLog{
			ID:                 primitive.NewObjectID(),
			InferenceServiceID: inferService.ID,
			CreatedAt:          time.Now().Unix(),
			Source:             r.Header.Get("X-Forwarded-For"),
			Revision:           lastRevision.Revision,
			InstanceID:         rw.Header().Get(InferenceResponseHeaderInstanceIDName),
			APIURL:             apiURL,
			Response:           err.Error(),
			Status:             http.StatusBadGateway,
		}
		AddInferenceServiceCallLog(colLog, logItem)
		InferenceRequestCountInc(col, serviceID, false)
		m.InferenceRequestCountMetricsInc(projectName, serviceID, logItem.InstanceID, http.StatusBadGateway)

		rw.WriteHeader(http.StatusBadGateway)
		log.WithError(err).Errorf("failed to invoking inference url host: %s, path: %s", inferService.ServiceInternalURI, apiURL)
	}

	c.Request.URL.Path = apiURL
	c.Request.URL.RawPath = apiURL
	c.Request.RequestURI = apiURL
	cleanProxyRequestHeader(c.Request)

	// intercept proxy request for logging
	utils.ServeHTTPReverseProxy(inferService.ServiceInternalURI, c.Writer, c.Request,
		func(proxy *httputil.ReverseProxy) {
			proxy.ModifyResponse = addInferenceServiceLog
		},
		func(proxy *httputil.ReverseProxy) {
			proxy.ErrorHandler = errorInferenceServiceLog
		},
	)
}

func (m *Mgr) ChatCompletion(c *gin.Context) {
	gc := ginlib.NewGinContext(c)
	projectName := gc.GetAuthProjectID()

	serviceID, revisionID, recordID, err := GetServiceIDRvIDAndRecordID(gc)
	if err != nil {
		ctx.Error(c, inferenceErr.ErrInvalidParam)
		return
	}

	service, err := FindInferenceSpecificRevision(m.getInferenceCollection(gc), gc.GetAuthProjectID(), serviceID, revisionID)
	if err != nil {
		log.WithError(err).Error("failed to FindInferenceSpecificRevision")
		ctx.Error(c, err)
		return
	}
	m.validateServiceStatus(service, false)
	if service.Revisions[0].State != types.InferenceServiceStateNormal {
		log.Errorf("inference revision is not normal: %+v", service.Revisions[0])
		ctx.Error(c, inferenceErr.ErrInvalidStateChange)
		return
	}
	record, err := FindConversationRecord(m.getConversationCollection(gc), recordID)
	if err != nil {
		log.WithError(err).Error("failed to FindConversationRecord record")
		ctx.Error(c, err)
		return
	}
	if record.TotalTokens > m.config.MaxToken {
		log.Error("no more conversation permitted")
		ctx.Error(c, inferenceErr.ErrInferenceMaxToken)
		return
	}
	if service.ServiceInternalURI == "" {
		log.Error("empty service internal URI")
		ctx.Error(c, inferenceErr.ErrInternal)
		return
	}
	urlstr, err := url.Parse(service.ServiceInternalURI)
	if err != nil {
		log.Errorf("invalid service uri: %s", service.ServiceInternalURI)
		ctx.Error(c, inferenceErr.ErrInternal)
		return
	}

	req := types.OpenAIChatReq{}
	if c.Request.Body != nil {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			log.Printf("Error reading request body: %v", err)
			return
		}
		if err := json.Unmarshal(body, &req); err != nil {
			log.WithError(err).Error("failed to unmarshal request body")
			ctx.Error(c, inferenceErr.ErrInvalidParam)
			return
		}
	}
	colConversation := m.getConversationCollection(gc)
	mergedConversation, err := FillHistoryConversationRecord(colConversation, service, recordID, &req)
	if err != nil {
		log.WithError(err).Error("failed to FillHistoryConversationRecord")
		ctx.Error(c, inferenceErr.ErrInternal)
		return
	}
	conversationIDs, token, err := ReserveUserContent(m.ossClient, colConversation, projectName, recordID, &req)
	if err != nil {
		log.WithError(err).Error("failed to ReserveUserContent")
		ctx.Error(c, err)
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(mergedConversation))
	c.Request.ContentLength = int64(len(mergedConversation))

	log.Infof("request log: host: %s, apiURL: %s", service.ServiceInternalURI, types.ChatAPIURL)
	logItemID := primitive.NewObjectID()
	colLog := m.getInferenceLogCollection(gc)
	colInfer := m.getInferenceCollection(gc)
	addInferenceServiceLog := func(resp *http.Response) error {
		log.Infof("response log: host: %s, path: %s, code: %d", service.ServiceInternalURI, types.ChatAPIURL, resp.StatusCode)
		logItem := &types.InferenceRequestLog{
			ID:                 logItemID,
			InferenceServiceID: serviceID,
			CreatedAt:          time.Now().Unix(),
			Source:             resp.Request.Header.Get("X-Forwarded-For"),
			Revision:           revisionID,
			InstanceID:         resp.Header.Get(InferenceResponseHeaderInstanceIDName),
			APIURL:             types.ChatAPIURL,
			Status:             resp.StatusCode,
		}
		AddInferenceServiceCallLog(colLog, logItem)
		InferenceRequestCountInc(colInfer, serviceID, resp.StatusCode == http.StatusOK)
		m.InferenceRequestCountMetricsInc(projectName, serviceID, "", resp.StatusCode)
		return nil
	}
	errorInferenceServiceLog := func(rw http.ResponseWriter, r *http.Request, err error) {
		log.WithError(err).Errorf("response error: host: %s, path: %s", service.ServiceInternalURI, types.ChatAPIURL)
		if err == context.Canceled {
			log.Errorf("http: proxy error: %v", err)
		} else {
			logItem := &types.InferenceRequestLog{
				InferenceServiceID: service.ID,
				CreatedAt:          time.Now().Unix(),
				Source:             r.Header.Get("X-Forwarded-For"),
				Revision:           revisionID,
				InstanceID:         rw.Header().Get(InferenceResponseHeaderInstanceIDName),
				APIURL:             "",
				Response:           err.Error(),
				Status:             http.StatusBadGateway,
			}
			AddInferenceServiceCallLog(colLog, logItem)
			InferenceRequestCountInc(colInfer, serviceID, false)
			m.InferenceRequestCountMetricsInc(projectName, serviceID, logItem.InstanceID, http.StatusBadGateway)
		}
		DeleteConversationFromConversationRecord(colConversation, recordID, conversationIDs, token)

		rw.WriteHeader(http.StatusBadGateway)
	}

	c.Request.URL.Path = types.ChatAPIURL
	c.Request.URL.RawPath = types.ChatAPIURL
	c.Request.RequestURI = types.ChatAPIURL
	c.Request.Host = urlstr.Host
	c.Request.Header.Set("X-Forwarded-Host", c.Request.Header.Get("Host"))

	proxy := httputil.NewSingleHostReverseProxy(urlstr)
	proxy.ModifyResponse = addInferenceServiceLog
	proxy.ErrorHandler = errorInferenceServiceLog
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		http.Error(c.Writer, "Streaming unsupported", http.StatusInternalServerError)
		return
	}
	rw := utils.InferenceChatResponseWriter{ResponseWriter: c.Writer, Buf: &bytes.Buffer{}, Flusher: flusher}
	rw.Header().Set("Cache-Control", "no-cache")
	rw.Header().Set("Connection", "keep-alive")
	proxy.ServeHTTP(rw, c.Request)

	responseData := GetResponseContent(rw.Buf.Bytes())
	if err := ReserveAssistantContent(colConversation, recordID, responseData); err != nil {
		log.WithError(err).Error("failed to ReserveAssistantContent")
	}
	if err := UpdateLogResponse(colLog, logItemID, string(responseData)); err != nil {
		log.WithError(err).Error("failed to UpdateLogResponse")
	}
	log.Infof("update response data successfully:%s, %s", responseData, logItemID.Hex())
}
