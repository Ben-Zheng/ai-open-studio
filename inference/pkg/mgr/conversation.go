package mgr

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/inference/pkg/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/inference/pkg/utils/ctx"
	inferenceErr "go.megvii-inc.com/brain/brainpp/projects/aiservice/inference/pkg/utils/ctx/errors"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/oss"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/utils/sds"
)

func (m *Mgr) CreateConversationRecord(c *gin.Context) {
	gc := ginlib.NewGinContext(c)
	serviceID, revisionID, err := GetServiceIDAndRvID(gc)
	if err != nil {
		ctx.Error(c, inferenceErr.ErrInvalidParam)
		return
	}
	req := types.CreateConversationRecordReq{}
	if err := gc.GetRequestJSONBody(&req); err != nil {
		log.WithError(err).Error("failed to unmarshal request body")
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
		ctx.Error(c, inferenceErr.ErrInferenceNotNormal)
		return
	}

	if !CheckTaskNameValid(req.Name) {
		log.Errorf("name is invalid: %s", req.Name)
		ctx.Error(c, inferenceErr.ErrInvalidParam)
		return
	}

	conversationRecord := CreateDefaultConversationRecord(req.Name, serviceID, revisionID)
	if err := InsertConversationRecord(m.getConversationCollection(gc), conversationRecord); err != nil {
		log.WithError(err).Error("failed to InserConversationRecord")
		ctx.Error(c, inferenceErr.ErrInternal)
		return
	}

	ctx.Success(c, conversationRecord)
}

func (m *Mgr) GetConversationRecord(c *gin.Context) {
	gc := ginlib.NewGinContext(c)
	serviceID, revisionID, recordID, err := GetServiceIDRvIDAndRecordID(gc)
	if err != nil {
		ctx.Error(c, inferenceErr.ErrInvalidParam)
		return
	}

	if _, err := FindInferenceSpecificRevision(m.getInferenceCollection(gc), gc.GetAuthProjectID(), serviceID, revisionID); err != nil {
		log.WithError(err).Error("failed to FindInferenceSpecificRevision")
		ctx.Error(c, err)
		return
	}

	record, err := FindConversationRecord(m.getConversationCollection(gc), recordID)
	if err != nil {
		log.WithError(err).Error("failed to FindConversationRecord")
		ctx.Error(c, err)
		return
	}

	if err := fillImageBase64Data(m.ossClient, record.Conversations); err != nil {
		log.WithError(err).Error("failed to FillImageBase64Data")
		ctx.Error(c, err)
		return
	}

	if record.TotalTokens > m.config.MaxToken {
		record.TobeMaxTokens = true
	}

	ctx.Success(c, record)
}

func (m *Mgr) ListConversationRecord(c *gin.Context) {
	gc := ginlib.NewGinContext(c)
	serviceID, revisionID, err := GetServiceIDAndRvID(gc)
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

	records, err := FindConversationRecordByIsvcRvID(m.getConversationCollection(gc), service.ID, revisionID)
	if err != nil {
		log.WithError(err).Error("failed to FindConversationRecordByIsvcRvID")
		return
	}

	ctx.Success(c, records)
}

func (m *Mgr) UpdateConversationRecord(c *gin.Context) {
	gc := ginlib.NewGinContext(c)
	serviceID, revisionID, recordID, err := GetServiceIDRvIDAndRecordID(gc)
	if err != nil {
		ctx.Error(c, inferenceErr.ErrInvalidParam)
		return
	}

	req := types.PatchConversationRecordReq{}
	if err := gc.GetRequestJSONBody(&req); err != nil {
		log.WithError(err).Error("failed to unmarshal request body")
		ctx.Error(c, inferenceErr.ErrInvalidParam)
		return
	}
	if !CheckTaskNameValid(req.Name) {
		log.Errorf("name is invalid: %s", req.Name)
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
		ctx.Error(c, inferenceErr.ErrInferenceNotNormal)
		return
	}

	if err := UpdateConversationRecordName(m.getConversationCollection(gc), recordID, req.Name); err != nil {
		log.WithError(err).Error("faield to updateConversationRecord")
		ctx.Error(c, inferenceErr.ErrInternal)
		return
	}
	record, err := FindConversationRecord(m.getConversationCollection(gc), recordID)
	if err != nil {
		log.WithError(err).Error("failed to FindConversationRecord")
		ctx.Error(c, err)
		return
	}

	ctx.Success(c, record)
}

func (m *Mgr) DeleteConversationRecord(c *gin.Context) {
	gc := ginlib.NewGinContext(c)
	serviceID, revisionID, recordID, err := GetServiceIDRvIDAndRecordID(gc)
	if err != nil {
		ctx.Error(c, inferenceErr.ErrInvalidParam)
		return
	}

	id, err := primitive.ObjectIDFromHex(recordID)
	if err != nil {
		log.WithError(err).Error("failed to get objectID")
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

	if err := DeleteConversationRecord(m.getConversationCollection(gc), id); err != nil {
		log.WithError(err).Error("faield to DeleteConversationRecord")
		ctx.Error(c, inferenceErr.ErrInternal)
		return
	}

	ctx.SuccessNoContent(c)
}

func CreateDefaultConversationRecord(name string, isvcID, isvcRevisionID string) *types.ConversationRecord {
	return &types.ConversationRecord{
		ID:                 primitive.NewObjectID(),
		InferenceServiceID: isvcID,
		IsvcRevisionID:     isvcRevisionID,
		Name:               name,
		Conversations:      []*types.Conversation{},
		IsDeleted:          false,
		CreatedAt:          time.Now().Unix(),
	}
}

func ReserveUserContent(ossClient *oss.Client, col *mongo.Collection, projectName, conversationRecordID string, chatReq *types.OpenAIChatReq) ([]primitive.ObjectID, int, error) {
	if chatReq == nil || len(chatReq.Messages) == 0 {
		return nil, 0, nil
	}

	id, err := primitive.ObjectIDFromHex(conversationRecordID)
	if err != nil {
		return nil, 0, err
	}

	var token int
	var conversations []*types.Conversation
	var conversationIDs []primitive.ObjectID
	for i := range chatReq.Messages {
		bs, _ := json.Marshal(chatReq.Messages[i])
		token += len(bs)
		switch content := chatReq.Messages[i].Content.(type) {
		case string:
			conversationID := primitive.NewObjectID()
			conversationIDs = append(conversationIDs, conversationID)
			conversations = append(conversations, &types.Conversation{
				ID:       conversationID,
				Role:     sds.UserRole,
				Modality: sds.TextModality,
				Content:  content,
			})
		case []interface{}:
			for _, c := range content {
				if v, ok := c.(map[string]interface{}); ok {
					conversationID := primitive.NewObjectID()
					conversationIDs = append(conversationIDs, conversationID)
					switch v["type"] {
					case "text":
						conversations = append(conversations, &types.Conversation{
							ID:       conversationID,
							Role:     sds.UserRole,
							Modality: sds.TextModality,
							Content:  v["text"].(string),
						})
					case "image_url":
						imageInfo, ok := v["image_url"].(map[string]interface{})
						if !ok {
							log.Errorf("assert image_url failed: %+v", v["image_url"])
							return nil, 0, inferenceErr.ErrInvalidParam
						}
						bucket := oss.GetOSSBucketName(projectName, oss.ComponentFileHub)
						key := "inference/" + conversationRecordID + "/" + GetRandomString(8)
						if err := uploadImageToS3(ossClient, bucket, key, imageInfo["url"].(string)); err != nil {
							log.Errorf("failed to upload to S3: %s, %s", bucket, key)
							return nil, 0, err
						}
						conversations = append(conversations, &types.Conversation{
							ID:       conversationID,
							Role:     sds.UserRole,
							Modality: sds.ImageModality,
							Content:  "s3://" + bucket + "/" + key,
						})
					default:
						log.Errorf("invalid content type: %+v", v)
						return nil, 0, inferenceErr.ErrInvalidParam
					}
				} else {
					log.Errorf("assert content item failed: %+v", c)
					return nil, 0, inferenceErr.ErrInvalidParam
				}
			}
		default:
			log.Errorf("assert message content failed: %+v", content)
			return nil, 0, inferenceErr.ErrInvalidParam
		}
	}

	return conversationIDs, token, PushConversationIntoConversationRecord(col, id, conversations, token)
}

func ReserveAssistantContent(col *mongo.Collection, conversationRecordID string, content []byte) error {
	if len(content) == 0 {
		return nil
	}

	id, err := primitive.ObjectIDFromHex(conversationRecordID)
	if err != nil {
		return err
	}
	return PushConversationIntoConversationRecord(col, id, []*types.Conversation{{
		ID:       primitive.NewObjectID(),
		Role:     sds.AssistantRole,
		Modality: sds.TextModality,
		Content:  string(content),
	}}, len(content))
}

func GetResponseContent(data []byte) []byte {
	var buf bytes.Buffer
	dataByte := []byte("data: ")
	nextLine := []byte("\n\n")
	lines := bytes.Split(data, dataByte)

	// 处理所有行
	for _, line := range lines {
		buf.Write(parseStreamData(bytes.TrimSuffix(line, nextLine)))
	}

	return buf.Bytes()
}

func parseStreamData(data []byte) []byte {
	if len(data) == 0 {
		return nil
	}

	stream := types.OpenAIChatRespChoiceStream{}
	if err := json.Unmarshal(data, &stream); err != nil {
		log.WithError(err).Errorf("failed to unmashal: {%s}", string(data))
		return nil
	}

	if len(stream.Choices) == 0 {
		log.Warningf("stream choices is empty: %+v", stream.Choices)
		return nil
	}

	if stream.Choices[0].FinishReason == "stop" || stream.Choices[0].Delta == nil {
		return nil
	}

	return []byte(stream.Choices[0].Delta.Content)
}

func fillImageBase64Data(c *oss.Client, conversations []*types.Conversation) error {
	for i := range conversations {
		if conversations[i].Modality == sds.ImageModality {
			ossURI := oss.NewOSSAbsoluteURI(conversations[i].Content)
			imageBytes, err := c.ReadObject(ossURI.GetBucket(), ossURI.GetKey())
			if err != nil {
				return err
			}
			conversations[i].Content = base64.StdEncoding.EncodeToString(imageBytes)
		}
	}

	return nil
}
