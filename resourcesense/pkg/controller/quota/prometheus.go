package quota

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"k8s.io/apiserver/pkg/server/healthz"

	aisTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types"
	quotaTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types/quota"
)

type HealthStatus float64

const (
	Unhealthy HealthStatus = iota
	Healthy
)

type PrometheusCollector struct {
	TotalCPU              prometheus.GaugeVec
	TotalMemory           prometheus.GaugeVec
	TotalGPU              prometheus.GaugeVec
	TotalVirtGPU          prometheus.GaugeVec
	TotalStorage          prometheus.GaugeVec
	TotalDatasetStorage   prometheus.GaugeVec
	ChargedCPU            prometheus.GaugeVec
	ChargedMemory         prometheus.GaugeVec
	ChargedGPU            prometheus.GaugeVec
	ChargedVirtGPU        prometheus.GaugeVec
	ChargedStorage        prometheus.GaugeVec
	ChargedDatasetStorage prometheus.GaugeVec
}

type RegistererGatherer interface {
	prometheus.Registerer
	prometheus.Gatherer
}

func NewPrometheusCollector() *PrometheusCollector {
	return &PrometheusCollector{
		TotalCPU: *prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "ais_tenantquota_total_cpu_cores",
			}, []string{"ais_tenant_name", "ais_project_name"}),
		TotalMemory: *prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "ais_tenantquota_total_memory_bytes",
			}, []string{"ais_tenant_name", "ais_project_name"}),
		TotalGPU: *prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "ais_tenantquota_total_gpu_cores",
			}, []string{"ais_tenant_name", "ais_project_name", "sub_type", "producer"}),
		TotalVirtGPU: *prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "ais_tenantquota_total_virtgpu_cores",
			}, []string{"ais_tenant_name", "ais_project_name", "sub_type", "producer"}),
		TotalStorage: *prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "ais_tenantquota_total_storage_bytes",
			}, []string{"ais_tenant_name", "ais_project_name"}),
		TotalDatasetStorage: *prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "ais_tenantquota_total_dataset_storage_bytes",
			}, []string{"ais_tenant_name", "ais_project_name"}),
		ChargedCPU: *prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "ais_tenantquota_charged_cpu_cores",
			}, []string{"ais_tenant_name", "ais_project_name"}),
		ChargedMemory: *prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "ais_tenantquota_charged_memory_bytes",
			}, []string{"ais_tenant_name", "ais_project_name"}),
		ChargedGPU: *prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "ais_tenantquota_charged_gpu_cores",
			}, []string{"ais_tenant_name", "ais_project_name", "sub_type", "producer"}),
		ChargedVirtGPU: *prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "ais_tenantquota_charged_virtgpu_cores",
			}, []string{"ais_tenant_name", "ais_project_name", "sub_type", "producer"}),
		ChargedStorage: *prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "ais_tenantquota_charged_storage_bytes",
			}, []string{"ais_tenant_name", "ais_project_name"}),
		ChargedDatasetStorage: *prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "ais_tenantquota_charged_dataset_storage_bytes",
			}, []string{"ais_tenant_name", "ais_project_name"}),
	}
}

// 如何做成动态，以及支持子类型资源
func (pc *PrometheusCollector) Describe(descs chan<- *prometheus.Desc) {
	pc.TotalCPU.Describe(descs)
	pc.TotalMemory.Describe(descs)
	pc.TotalGPU.Describe(descs)
	pc.TotalVirtGPU.Describe(descs)
	pc.TotalStorage.Describe(descs)
	pc.TotalDatasetStorage.Describe(descs)
	pc.ChargedCPU.Describe(descs)
	pc.ChargedMemory.Describe(descs)
	pc.ChargedGPU.Describe(descs)
	pc.ChargedVirtGPU.Describe(descs)
	pc.ChargedStorage.Describe(descs)
	pc.ChargedDatasetStorage.Describe(descs)
}

func (pc *PrometheusCollector) Collect(c chan<- prometheus.Metric) {
	pc.TotalCPU.Collect(c)
	pc.TotalMemory.Collect(c)
	pc.TotalGPU.Collect(c)
	pc.TotalVirtGPU.Collect(c)
	pc.TotalStorage.Collect(c)
	pc.TotalDatasetStorage.Collect(c)
	pc.ChargedCPU.Collect(c)
	pc.ChargedMemory.Collect(c)
	pc.ChargedGPU.Collect(c)
	pc.ChargedVirtGPU.Collect(c)
	pc.ChargedStorage.Collect(c)
	pc.ChargedDatasetStorage.Collect(c)
}

func (pc *PrometheusCollector) Update(tenant, project string, totalQuota, chargedQuota quotaTypes.Quota) {
	pc.TotalCPU.WithLabelValues(tenant, project).Set(float64(totalQuota.CPU()))
	pc.TotalMemory.WithLabelValues(tenant, project).Set(float64(totalQuota.Memory()))
	pc.TotalGPU.WithLabelValues(tenant, project, "", "").Set(float64(totalQuota.GPU() + totalQuota.NPU()))
	pc.TotalVirtGPU.WithLabelValues(tenant, project, "", "").Set(float64(totalQuota.VirtGPU()))
	pc.TotalStorage.WithLabelValues(tenant, project).Set(float64(totalQuota.Storage()))
	pc.TotalDatasetStorage.WithLabelValues(tenant, project).Set(float64(totalQuota.DatasetStorage()))
	pc.ChargedCPU.WithLabelValues(tenant, project).Set(float64(chargedQuota.CPU()))
	pc.ChargedMemory.WithLabelValues(tenant, project).Set(float64(chargedQuota.Memory()))
	pc.ChargedGPU.WithLabelValues(tenant, project, "", "").Set(float64(chargedQuota.GPU() + chargedQuota.NPU()))
	pc.ChargedVirtGPU.WithLabelValues(tenant, project, "", "").Set(float64(chargedQuota.VirtGPU()))
	pc.ChargedStorage.WithLabelValues(tenant, project).Set(float64(chargedQuota.Storage()))
	pc.ChargedDatasetStorage.WithLabelValues(tenant, project).Set(float64(chargedQuota.DatasetStorage()))

	for k, v := range totalQuota {
		if types.IsGPUResourceName(k) {
			pc.TotalGPU.WithLabelValues(tenant, project, "", aisTypes.NVIDIAProducer).Set(float64(v.Value()))
			pc.TotalGPU.WithLabelValues(tenant, project, types.GPUTypeFromResourceName(k), aisTypes.NVIDIAProducer).Set(float64(v.Value()))
		} else if types.IsNPUResourceName(k) {
			pc.TotalGPU.WithLabelValues(tenant, project, "", aisTypes.HUAWEIProducer).Set(float64(v.Value()))
			pc.TotalGPU.WithLabelValues(tenant, project, types.NPUTypeFromResourceName(k), aisTypes.HUAWEIProducer).Set(float64(v.Value()))
		}
	}
	for k, v := range chargedQuota {
		if types.IsGPUResourceName(k) {
			pc.ChargedGPU.WithLabelValues(tenant, project, "", aisTypes.NVIDIAProducer).Set(float64(v.Value()))
			pc.ChargedGPU.WithLabelValues(tenant, project, types.GPUTypeFromResourceName(k), aisTypes.NVIDIAProducer).Set(float64(v.Value()))
		} else if types.IsNPUResourceName(k) {
			pc.ChargedGPU.WithLabelValues(tenant, project, "", aisTypes.HUAWEIProducer).Set(float64(v.Value()))
			pc.ChargedGPU.WithLabelValues(tenant, project, types.NPUTypeFromResourceName(k), aisTypes.HUAWEIProducer).Set(float64(v.Value()))
		}
	}

	for k, v := range totalQuota {
		if types.IsVirtGPUResourceName(k) {
			pc.TotalVirtGPU.WithLabelValues(tenant, project, "", aisTypes.NVIDIAProducer).Set(float64(v.Value()))
			pc.TotalVirtGPU.WithLabelValues(tenant, project, types.VirtGPUTypeFromResourceName(k), aisTypes.NVIDIAProducer).Set(float64(v.Value()))
		}
	}
	for k, v := range chargedQuota {
		if types.IsVirtGPUResourceName(k) {
			pc.ChargedVirtGPU.WithLabelValues(tenant, project, "", aisTypes.NVIDIAProducer).Set(float64(v.Value()))
			pc.ChargedVirtGPU.WithLabelValues(tenant, project, types.VirtGPUTypeFromResourceName(k), aisTypes.NVIDIAProducer).Set(float64(v.Value()))
		}
	}
}

type OnDemandCollector struct {
	*PrometheusCollector
}

func NewOnDemandCollector() *OnDemandCollector {
	return &OnDemandCollector{
		NewPrometheusCollector(),
	}
}

func (c *OnDemandCollector) Collect(m chan<- prometheus.Metric) {
	tenants, err := ListAllTenantDetails()
	if err != nil {
		log.WithError(err).Error("failed to ListAllTenantDetails")
		return
	}
	for _, tenant := range tenants {
		c.PrometheusCollector.Update(tenant.TenantID, "", tenant.TotalQuota, tenant.ChargedQuota)
	}
	c.PrometheusCollector.Collect(m)
}

func (c *OnDemandCollector) Describe(descs chan<- *prometheus.Desc) {
	c.PrometheusCollector.Describe(descs)
}

func RegisterMetricsServer(bindAddr string, registry RegistererGatherer, checkers ...healthz.HealthChecker) (*http.Server, error) {
	var handler http.Handler
	if registry != nil {
		handler = promhttp.InstrumentMetricHandler(registry, promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	} else {
		handler = promhttp.Handler()
	}
	// Setup metrics and healthz server
	mux := http.NewServeMux()
	mux.Handle("/metrics", handler)

	// Run livez server
	mux.HandleFunc("/livez", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// Run healthz server
	healthz.InstallHandler(mux, checkers...)
	return &http.Server{
		Addr:    bindAddr,
		Handler: mux,
	}, nil
}

func RegisterHealthGaugeFunc(checkers ...healthz.HealthChecker) *prometheus.Registry {
	registry := prometheus.NewPedanticRegistry()
	registry.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "health_check",
		Help: "return 1 means healthy, 0 and others mean unhealthy",
	}, func() float64 {
		r, err := http.NewRequest("GET", "/healthz", nil)
		if err != nil {
			log.WithError(err).Errorf("new request in RegisterHealthGaugeFunc")
			return float64(Unhealthy)
		}
		for _, c := range checkers {
			if err := c.Check(r); err != nil {
				return float64(Unhealthy)
			}
		}
		return float64(Healthy)
	}))
	return registry
}
