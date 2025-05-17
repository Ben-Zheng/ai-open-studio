package utils

import (
	"sync"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/config"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/utils/fluentbit"
)

const (
	LogFieldFailedReason  = "failedReason"
	LogFieldRvCreatedAt   = "revisionCreatedAt"
	LogFieldCompletedWay  = "completedWay" // 归入成功状态的方式
	LogFieldSnapXImageURI = "snapXImageURI"
	LogFieldManagedBy     = "managedBy"

	CompletedWayNormal               = "AutoComplete"   // 全程自动正常结束
	CompletedWayManualStopAfLearning = "StopAfLearning" // 进入学习后手动停止
	CompletedWayManualStopBfLearning = "StopBfLearning" // 未进入学习前手动停止
)

var autoLearnMetricLog sync.Once
var autoLearnMetricLogClient *fluentbit.Client

func getAutoLearnMetricLoggerClient() *fluentbit.Client {
	autoLearnMetricLog.Do(func() {
		autoLearnMetricLogClient = fluentbit.NewClient(&fluentbit.Options{
			Addr:   config.Profile.FluentbitServer,
			TagSet: fluentbit.MetricEventTagSet,
		})
	})
	return autoLearnMetricLogClient
}

func MetricEventDump(autoLearnID string, revisionID string, appType types.AppType, metricEventType string, extra map[string]any) {
	event := map[string]any{
		"serviceType":     types.AIServiceTypeAutoLearn,
		"appType":         appType,
		"autoLearnID":     autoLearnID,
		"revisionID":      revisionID,
		"metricEventType": metricEventType,
	}
	for k, v := range extra {
		if k == LogFieldFailedReason && v == "" {
			v = "unknown error"
		}
		event[k] = v
	}

	getAutoLearnMetricLoggerClient().Dump(event)
}
