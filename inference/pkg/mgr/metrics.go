package mgr

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/penglongli/gin-metrics/ginmetrics"
)

const (
	InferenceRequestTotalMetrics = "inference_request_total"
)

func (m *Mgr) RegisterMetricsService(r *gin.Engine) *ginmetrics.Monitor {
	montior := ginmetrics.GetMonitor()
	montior.SetMetricPath("/metrics")
	montior.Use(r)

	m.addInferenceRequestMetrics(montior)

	return montior
}

func (m *Mgr) addInferenceRequestMetrics(metrics *ginmetrics.Monitor) error {
	totalInferRequestMetric := &ginmetrics.Metric{
		Type:        ginmetrics.Counter,
		Name:        InferenceRequestTotalMetrics,
		Description: "all the server received inference request num.",
		Labels:      []string{"project_name", "inference_service_id", "instance_id", "status_code"},
	}

	// Add metric to global monitor object
	return metrics.AddMetric(totalInferRequestMetric)
}

func (m *Mgr) InferenceRequestCountMetricsInc(projectName, serviceID, instanceID string, code int) {
	ginmetrics.GetMonitor().GetMetric(InferenceRequestTotalMetrics).Inc([]string{projectName, serviceID, instanceID, fmt.Sprintf("%d", code)})
}
