package quota

import (
	"fmt"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	k8sresource "k8s.io/apimachinery/pkg/api/resource"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/features"
	aisUtils "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/utils"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types/resource"
)

type QuotaData map[types.KubeResourceName]float64 // nolint

type Quota corev1.ResourceList // 新版本的 Quota 结构，支持动态资源名称

func (q Quota) ToData() QuotaData {
	ret := NilQuotaData()
	gpuTypeEnabled := features.IsQuotaSupportGPUGroupEnabled()
	for k, v := range q {
		if !gpuTypeEnabled && (types.IsGPUResourceName(k) || types.IsVirtGPUResourceName(k)) {
			continue
		}
		ret[k] = float64(v.Value())
	}
	return ret
}

func BuildQuotaFromData(qd QuotaData) Quota {
	ret := NilQuota()
	for k, v := range qd {
		ret[k] = k8sresource.MustParse(strconv.FormatInt(int64(v), 10))
	}
	return ret
}

func (q Quota) CPU() int64 {
	return q.Get(types.KubeResourceCPU)
}

func (q Quota) Memory() int64 {
	return q.Get(types.KubeResourceMemory)
}

func (q Quota) GPU() int64 {
	return q.Get(types.KubeResourceGPU)
}

func (q Quota) NPU() int64 {
	if q == nil {
		return 0
	}
	return q.Get(types.KubeResourceNPU)
}

func (q Quota) SumNPUType() int64 {
	var total int64
	for k, v := range q {
		if types.IsNPUResourceName(k) {
			total += v.Value()
		}
	}
	return total
}

func (q Quota) DatasetStorage() int64 {
	return q.Get(types.KubeResourceDatasetStorage)
}

func (q Quota) SumGPUType() int64 {
	var total int64
	for k, v := range q {
		if types.IsGPUResourceName(k) {
			total += v.Value()
		}
	}
	return total
}

func (q Quota) SumVirtGPUType() int64 {
	var total int64
	for k, v := range q {
		if types.IsVirtGPUResourceName(k) {
			total += v.Value()
		}
	}
	return total
}

func (q Quota) VirtGPU() int64 {
	return q.Get(types.KubeResourceVirtGPU)
}

func (q Quota) Storage() int64 {
	return q.Get(types.KubeResourceStorage)
}

func (q Quota) Get(resourceName corev1.ResourceName) int64 {
	if val, ok := q[resourceName]; ok {
		return val.Value()
	}
	return 0
}

func (q Quota) LessThan(o Quota) bool {
	for k, v := range q {
		if v.Cmp(o[k]) > 0 {
			return false
		}
	}
	return true
}

func (q Quota) InplaceAdd(o Quota) {
	for k, v := range o {
		if quantity, ok := q[k]; ok {
			v.Add(quantity)
		}
		q[k] = v
	}
}

func (q Quota) InplaceScala(scale int32) {
	for k, v := range q {
		v.ScaledValue(k8sresource.Scale(scale))
		q[k] = v
	}
}

func (q Quota) InplaceSub(o Quota) {
	for k, v := range o {
		quantity := q[k]
		quantity.Sub(v)
		q[k] = quantity
	}
}

func (q Quota) ReserveResource(o Quota, reserveNames []corev1.ResourceName) {
	for _, resourceName := range reserveNames {
		q.SetByResourceName(resourceName, o.GetByResourceName(resourceName))
	}
}
func (q Quota) Overwrite(o Quota, include, exclude []corev1.ResourceName) {
	if len(include) > 0 {
		for _, resourceName := range include {
			q.SetByResourceName(resourceName, o.GetByResourceName(resourceName))
		}
	}
	if len(exclude) > 0 {
		excludes := make(map[corev1.ResourceName]bool)
		for _, key := range exclude {
			excludes[key] = true
		}
		for resourceName := range o {
			if found := excludes[resourceName]; found {
				continue
			}
			q.SetByResourceName(resourceName, o.GetByResourceName(resourceName))
		}
	}
}

func (q Quota) Add(o Quota) Quota {
	ret := NilQuota()
	ret.InplaceAdd(q)
	ret.InplaceAdd(o)
	return ret
}

func (q Quota) Sub(o Quota) Quota {
	ret := NilQuota()
	ret.InplaceAdd(q)
	ret.InplaceSub(o)
	return ret
}

func (q Quota) Minus(factor float64) Quota {
	ret := NilQuota()
	ret.InplaceAdd(q)
	for k, v := range ret {
		// 数据集存储配额不支持超发
		if k == types.KubeResourceDatasetStorage {
			continue
		}
		v.Set(int64(factor) * v.Value())
		ret[k] = v
	}
	return ret
}

func (q Quota) NoNegative() Quota {
	ret := NilQuota()
	ret.InplaceAdd(q)
	for k, v := range ret {
		if v.CmpInt64(0) <= 0 {
			ret[k] = *k8sresource.NewQuantity(0, v.Format)
		}
	}
	return ret
}

func (q Quota) IsZero() bool {
	for _, v := range q {
		if !v.IsZero() {
			return false
		}
	}
	return true
}

func (q Quota) GetByResourceName(resourceName corev1.ResourceName) k8sresource.Quantity {
	return q[resourceName]
}

func (q Quota) SetByResourceName(resourceName corev1.ResourceName, quantity k8sresource.Quantity) {
	q[resourceName] = quantity
}

func (q Quota) String() string {
	resources := []string{}
	for k, v := range q {
		if v.Format == k8sresource.BinarySI {
			resources = append(resources, fmt.Sprintf("%s: %d", k, aisUtils.ByteToGB(v.Value())))
		} else {
			resources = append(resources, fmt.Sprintf("%s: %d", k, v.Value()))
		}
	}
	return fmt.Sprintf("Quota(%s)", strings.Join(resources, ", "))
}

func (qd QuotaData) ToMongo() QuotaData {
	ret := QuotaData{}
	for k, v := range qd {
		ret[corev1.ResourceName(EncodeMongoKeyword(string(k)))] = v
	}
	return ret
}

func (qd QuotaData) FromMongo() QuotaData {
	ret := NilQuotaData()
	for k, v := range qd {
		if !features.IsQuotaSupportGPUGroupEnabled() {
			if types.IsGPUResourceName(corev1.ResourceName(DecodeMongoKeyword(string(k)))) {
				continue
			}
		}
		ret[corev1.ResourceName(DecodeMongoKeyword(string(k)))] = v
	}
	return ret
}

func (qd QuotaData) String() string {
	resources := []string{}
	for k, v := range qd {
		if k == types.ResourceMemory || k == types.ResourceStorage {
			resources = append(resources, fmt.Sprintf("%s: %d", k, aisUtils.ByteToGB(int64(v))))
		} else {
			resources = append(resources, fmt.Sprintf("%s: %d", k, int64(v)))
		}
	}
	return fmt.Sprintf("QuotaData(%s)", strings.Join(resources, ", "))
}

func NilQuota() Quota {
	return Quota{
		types.KubeResourceCPU:            k8sresource.Quantity{},
		types.KubeResourceGPU:            k8sresource.Quantity{},
		types.KubeResourceNPU:            k8sresource.Quantity{},
		types.KubeResourceVirtGPU:        k8sresource.Quantity{},
		types.KubeResourceMemory:         k8sresource.Quantity{},
		types.KubeResourceStorage:        k8sresource.Quantity{},
		types.KubeResourceDatasetStorage: k8sresource.Quantity{},
	}
}

func NilQuotaData() QuotaData {
	return QuotaData{
		types.KubeResourceCPU:            0,
		types.KubeResourceGPU:            0,
		types.KubeResourceNPU:            0,
		types.KubeResourceVirtGPU:        0,
		types.KubeResourceMemory:         0,
		types.KubeResourceStorage:        0,
		types.KubeResourceDatasetStorage: 0,
	}
}

func BuildQuotaFromUsed(used *resource.Used) Quota {
	return Quota{
		types.KubeResourceCPU:            *k8sresource.NewQuantity(int64(used.CPU*1000)/1000, k8sresource.DecimalSI),
		types.KubeResourceGPU:            *k8sresource.NewQuantity(int64(used.GPU*1000)/1000, k8sresource.DecimalSI),
		types.KubeResourceNPU:            *k8sresource.NewQuantity(int64(used.NPU*1000)/1000, k8sresource.DecimalSI),
		types.KubeResourceVirtGPU:        *k8sresource.NewQuantity(int64(used.VirtGPU*1000)/1000, k8sresource.DecimalSI),
		types.KubeResourceMemory:         *k8sresource.NewQuantity(int64(used.Memory*1000)/1000, k8sresource.BinarySI),
		types.KubeResourceStorage:        *k8sresource.NewQuantity(int64(used.Storage*1000)/1000, k8sresource.BinarySI),
		types.KubeResourceDatasetStorage: *k8sresource.NewQuantity(0, k8sresource.BinarySI),
	}
}

func EncodeMongoKeyword(key string) string {
	return strings.ReplaceAll(key, ".", "@")
}

func DecodeMongoKeyword(key string) string {
	ret := strings.ReplaceAll(key, "@", ".")
	return ret
}
