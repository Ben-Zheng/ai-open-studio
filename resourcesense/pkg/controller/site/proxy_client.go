package site

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	authTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/auth/pkg/app/types"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/ginlib"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/rest"
	aisTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/types"
	aisUtils "go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/utils"
	quotaTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types/quota"
	siteTypes "go.megvii-inc.com/brain/brainpp/projects/aiservice/resourcesense/pkg/types/site"
)

const (
	defaultRequestTimeout = 120
)

type SitesClusterQuota struct {
	Overview     *quotaTypes.ClusterQuota            `json:"overview"`
	Distribution map[string]*quotaTypes.ClusterQuota `json:"distribution"`
}

type SitesClusterAPU struct {
	Distribution map[string][]*aisTypes.APUConfig `json:"distribution"`
}

type Client interface {
	Proxy(gc *ginlib.GinContext, sites []*siteTypes.Site) (interface{}, error)
	SitesClusterQuota(gc *ginlib.GinContext, sites []*siteTypes.Site) (*SitesClusterQuota, error)
	SitesClusterAPU(gc *ginlib.GinContext, sites []*siteTypes.Site) (*SitesClusterAPU, error)
	SitesTenantQuota(gc *ginlib.GinContext, sites []*siteTypes.Site) (*SitesClusterQuota, error)
	ReportStatus(gc *ginlib.GinContext, self, center *siteTypes.Site) error
}

type Endpoint struct {
	URI           string
	APIPrefix     string
	RewritePrefix string
	SortMapper    map[string]string // 将排序字段映射为底层的key
}

var defaultSortMapper = map[string]string{
	"createdAt":         "createdAt",
	"updatedAt":         "updatedAt",
	"timestamp":         "timestamp",
	"creationTimestamp": "creationTimestamp",
	"name":              "name",
}

var EndpointAPIPrefixes = map[string]*Endpoint{
	"datahub-clean": {
		URI:           "http://datahub-clean-http.svc",
		APIPrefix:     "/aapi/datahub.ais.io/api/v1/clean/",
		RewritePrefix: "/api/v1/clean/",
	},
	"datahub-apiserver": {
		URI:           "http://datahub-apiserver-http.svc",
		APIPrefix:     "/aapi/datahub.ais.io/",
		RewritePrefix: "/",
	},
	"datahub-ailabel": {
		URI:           "http://ailabel-http.svc",
		APIPrefix:     "/aapi/ailabel.ais.io/",
		RewritePrefix: "/",
	},
	"datahub-basiclabel": {
		URI:           "http://basiclabel-http.svc",
		APIPrefix:     "/aapi/basiclabel.ais.io/",
		RewritePrefix: "/",
	},
	"codehub": {
		URI:           "http://codehub-http.svc",
		APIPrefix:     "/aapi/codehub.ais.io/",
		RewritePrefix: "/",
	},
	"autolearn": {
		URI:           "http://autolearn-http.svc",
		APIPrefix:     "/aapi/autolearn.ais.io/",
		RewritePrefix: "/",
	},
	"modelhub": {
		URI:           "http://modelhub-http.svc",
		APIPrefix:     "/aapi/modelhub.ais.io/",
		RewritePrefix: "/",
	},
	"evalhub": {
		URI:           "http://evalhub-http.svc",
		APIPrefix:     "/aapi/evalhub.ais.io/",
		RewritePrefix: "/",
	},
	"inference": {
		URI:           "http://inference-http.svc",
		APIPrefix:     "/aapi/inference.ais.io/",
		RewritePrefix: "/",
	},
	"algohub": {
		URI:           "http://algohub-http.svc",
		APIPrefix:     "/aapi/algohub.ais.io/",
		RewritePrefix: "/",
	},
	"resourcesense": {
		URI:           "http://resourcesense-http.svc",
		APIPrefix:     "/aapi/resourcesense.ais.io/",
		RewritePrefix: "/",
	},
	"aworkspace": {
		URI:           "http://workspace-http.svc",
		APIPrefix:     "/aapi/workspace.ais.io/",
		RewritePrefix: "/",
	},
	"publicservice": {
		URI:           "http://publicservice-http.svc",
		APIPrefix:     "/aapi/publicservice.ais.io/",
		RewritePrefix: "/",
	},
}

type client struct {
	EndPoint *aisTypes.EndPoint `json:"endpoint" yaml:"endpoint"`
	client   *rest.Client
}

func NewClient(endpoint *aisTypes.EndPoint) Client {
	return &client{
		EndPoint: endpoint,
		client:   rest.NewClient(),
	}
}

func (c *client) buildRequest(gc *ginlib.GinContext, endpoint *Endpoint, url string, pagination *ginlib.Pagination) (*http.Request, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.WithError(err).Error("failed to release by user request")
		return nil, err
	}
	q := req.URL.Query()
	rawQuery := gc.Request.URL.Query()
	for k, values := range rawQuery {
		q[k] = values
	}

	// 算法仓特殊处理
	if endpoint.APIPrefix == EndpointAPIPrefixes["algohub"].APIPrefix {
		q["noAllocatedFirst"] = []string{"true"}
	}

	// 设置分页参数
	if pagination != nil {
		req.Header.Set(ginlib.HeaderPageSkip, fmt.Sprint(pagination.Skip))
		req.Header.Set(authTypes.AISPageSkipHeader, fmt.Sprint(pagination.Skip))
		req.Header.Set(ginlib.HeaderPageLimit, fmt.Sprint(pagination.Limit))
		req.Header.Set(authTypes.AISPageLimitHeader, fmt.Sprint(pagination.Limit))
	}
	req.Header.Set(authTypes.AISAuthHeader, gc.GetAuthToken())
	req.Header.Set(authTypes.AISProjectHeader, gc.GetAuthProjectID())
	req.Header.Set(authTypes.AISTenantHeader, gc.GetAuthTenantID())
	req.Header.Set(authTypes.AISSubjectNameHeader, gc.GetUserName())
	req.Header.Set(authTypes.AISSubjectHeader, gc.GetUserID())
	req.Header.Set(authTypes.AISSourceLevel, gc.GetSourceLevel())

	req.Header.Set("Content-Type", "application/json")
	req.URL.RawQuery = q.Encode()
	return req, nil
}

func (c *client) BuildSiteRequest(gc *ginlib.GinContext, site *siteTypes.Site, moduleapi string, pt PageToken) (*http.Request, error) {
	endpoint := c.matchEndpoint(moduleapi)
	if endpoint == nil {
		return nil, fmt.Errorf("fuck no match: %s", moduleapi)
	}
	logger := log.WithFields(log.Fields{
		"site":   site.Name,
		"domain": site.Domain,
	})

	useAdmin := false
	if strings.Contains(gc.Request.Host, "admin") {
		useAdmin = true
	}
	uri := c.buildSiteURI(site, endpoint, moduleapi, false, useAdmin)
	logger.Infof("BuildSiteRequest endpoint: %s, moduleapi: %s, uri: %s, site: %+v", endpoint, moduleapi, uri, site)
	var pagination *ginlib.Pagination
	if pt != nil {
		pagination = c.parseSitePagination(site, pt, gc.GetPagination())
		logger.Infof("parse site pagination, got: %+v", pagination)
	}
	req, err := c.buildRequest(gc, endpoint, uri, pagination)
	if err != nil {
		return nil, err
	}
	return req, err
}

func (c *client) DoRequest(gc *ginlib.GinContext, req *http.Request) ([]byte, interface{}, error) {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}
	resp, err := client.Do(req.WithContext(gc.Context))
	if err != nil {
		log.WithError(err).Error("release by user failed")
		return nil, nil, err
	}

	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("request with status code: %d", resp.StatusCode)
		log.WithError(err).WithField("request", req).Error("request failed ")
		return nil, nil, err
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error("failed to read response body")
		return nil, nil, err
	}

	var data interface{}
	if err = json.Unmarshal(body, &data); err != nil {
		log.WithFields(log.Fields{"status code": resp.StatusCode, "body": string(body)}).WithError(err).Error("failed to unmarshal response body")
		return nil, nil, err
	}
	return body, data, err
}

func injectSiteToItems(site *siteTypes.Site, items interface{}, sorter *ginlib.Sort) []*Payload {
	ret := make([]*Payload, 0)
	if items == nil {
		return ret
	}
	v := reflect.ValueOf(items)
	if v.Kind() == reflect.Slice {
		for i := 0; i < v.Len(); i++ {
			strct := v.Index(i).Interface()
			sv := reflect.ValueOf(strct)

			var weight int64
			if sv.Kind() == reflect.Map {
				if sorter != nil && sorter.By != "" {
					for _, key := range sv.MapKeys() {
						if key.Interface().(string) == sorter.By {
							switch t := sv.MapIndex(key).Interface().(type) {
							case int64:
								weight = sv.MapIndex(key).Interface().(int64)
							case int:
								weight = int64(sv.MapIndex(key).Interface().(int))
							case float64:
								weight = int64(sv.MapIndex(key).Interface().(float64))
							default:
								log.Errorf("unexcepted weight key, %s, type: %+v", sorter.By, reflect.TypeOf(t))
							}
						}
					}
				}
				sv.SetMapIndex(reflect.ValueOf("site"), reflect.ValueOf(site.Name))
				sv.SetMapIndex(reflect.ValueOf("siteDisplayName"), reflect.ValueOf(site.DisplayName))
			}

			ret = append(ret, &Payload{
				Value:  strct,
				Weight: weight,
				Site:   site,
			})
		}
	}
	return ret
}

func MapToMemorySorter(endpoint *Endpoint, sorter *ginlib.Sort) *ginlib.Sort {
	mapper := defaultSortMapper
	if sorter == nil {
		return nil
	}

	if endpoint.SortMapper != nil {
		mapper = endpoint.SortMapper
	}

	if field, ok := mapper[sorter.By]; ok {
		return &ginlib.Sort{
			By:    field,
			Order: sorter.Order,
		}
	}
	return nil
}

// bj:1~2+sh:3~6
type PageOffset struct {
	Start int
	End   int
}
type PageToken map[string]PageOffset

func (pt PageToken) Encode() string {
	offsets := make([]string, 0)
	for site, offset := range pt {
		offsets = append(offsets, fmt.Sprintf("%s:%d~%d", site, offset.Start, offset.End))
	}
	return strings.Join(offsets, "+")
}

func (pt PageToken) Add(other PageToken) PageToken {
	res := make(PageToken)
	for site, offset := range pt {
		res[site] = offset
	}
	for site, offset := range other {
		pt, ok := res[site]
		if !ok {
			pt = PageOffset{}
		}
		pt.Start = pt.End
		pt.End += offset.End
		res[site] = pt
	}
	return res
}

func (pt PageToken) Sub(other PageToken) PageToken {
	res := make(PageToken)
	for site, offset := range pt {
		res[site] = offset
	}
	for site, offset := range other {
		pt, ok := res[site]
		if !ok {
			pt = PageOffset{}
		}
		pt.Start -= offset.End
		pt.End = pt.Start + offset.End
		res[site] = pt
	}
	return res
}

func (pt PageToken) Page(pg *ginlib.Pagination) int {
	count := 0
	size := int(pg.Limit)
	for _, offset := range pt {
		count += offset.End
	}
	ptPage := (count + size - 1) / size
	return ptPage
}

func (pt PageToken) IsCurrent(pg *ginlib.Pagination) bool {
	return pt.Page(pg) == int(pg.GetPage())
}

func (pt PageToken) IsNext(pg *ginlib.Pagination) bool {
	return pt.Page(pg) == int(pg.GetPage())-1
}

func (pt PageToken) IsPrevious(pg *ginlib.Pagination) bool {
	return pt.Page(pg) == int(pg.GetPage())+1
}

func DecodePageToken(pagetoken string) PageToken {
	pt := make(PageToken)
	offsets := strings.Split(pagetoken, "+")
	for i := range offsets {
		offset := strings.Split(offsets[i], ":")
		if len(offset) != 2 {
			continue
		}

		offsetsplits := strings.Split(offset[1], "~")
		if len(offsetsplits) != 2 {
			continue
		}

		start, err := strconv.ParseInt(offsetsplits[0], 10, 64)
		if err != nil {
			continue
		}
		end, err := strconv.ParseInt(offsetsplits[1], 10, 64)
		if err != nil {
			continue
		}
		pt[offset[0]] = PageOffset{
			Start: int(start),
			End:   int(end),
		}
	}
	return pt
}

type DynamicResponse struct {
	Value interface{}
	Raw   json.RawMessage
	Site  *siteTypes.Site
}

func DetermineDynamicResponseType(dr *DynamicResponse) (t ResponseDataType) {
	v := reflect.ValueOf(dr.Value)

	if v.Kind() == reflect.Slice {
		return ResponseDataTypeC
	} else if v.Kind() == reflect.Map {
		var hasData, hasTotal, hasAutoLearns, hasDataItems bool
		for _, key := range v.MapKeys() {
			keyValue := key.Interface().(string)
			if keyValue == "data" {
				hasData = true
				strct := v.MapIndex(key).Interface()
				sv := reflect.ValueOf(strct)
				if sv.Kind() == reflect.Map {
					for _, skey := range sv.MapKeys() {
						if skey.Interface().(string) == "items" {
							hasDataItems = true
						}
					}
				}
			} else if keyValue == "total" {
				hasTotal = true
			} else if keyValue == "autoLearns" {
				hasAutoLearns = true
			}
		}

		if hasData && hasTotal {
			return ResponseDataTypeA
		} else if hasAutoLearns && hasTotal {
			return ResponseDataTypeD
		} else if hasDataItems {
			return ResponseDataTypeB
		}
	}
	return ResponseDataType0
}

func GetValueFromMap(data interface{}, findKey string) interface{} {
	v := reflect.ValueOf(data)
	if v.Kind() == reflect.Map {
		for _, key := range v.MapKeys() {
			keyValue := key.Interface().(string)
			if keyValue == findKey {
				return v.MapIndex(key).Interface()
			}
		}
	}
	return nil
}

func GetInt64FromMap(data interface{}, findKey string) int64 {
	value := GetValueFromMap(data, findKey)
	if value == nil {
		return 0
	}

	switch v := value.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	default:
		log.Errorf("Get int64 value type: %+v", reflect.TypeOf(value))
	}
	return 0
}

type ResponseDataType string

const (
	ResponseDataType0 ResponseDataType = "0" // unknown
	ResponseDataTypeA ResponseDataType = "A" // map[data + total]
	ResponseDataTypeB ResponseDataType = "B" // map[data: map[items + total]]
	ResponseDataTypeC ResponseDataType = "C" // slice
	ResponseDataTypeD ResponseDataType = "D" // map[autoLearns + total]
)

const (
	OrderByCreatedAt string = "createdAt"
	OrderByUpdatedAt string = "updatedAt"
)

type Payload struct {
	Value  interface{}
	Weight int64
	Site   *siteTypes.Site
}

func (c *client) MergeSiteResponses(siteResponses map[string]*DynamicResponse, sorter *ginlib.Sort, previouspt PageToken, pg *ginlib.Pagination) (interface{}, error) {
	payloads := make(map[string][]*Payload, 0)
	items := make([]interface{}, 0)
	var total int64
	var response interface{}
	t := ResponseDataType0
	for _, siteResponse := range siteResponses {
		t = DetermineDynamicResponseType(siteResponse)
		break
	}

	switch t {
	case ResponseDataTypeA:
		for site, dr := range siteResponses {
			items := GetValueFromMap(dr.Value, "data")
			payload := injectSiteToItems(dr.Site, items, sorter)
			payloads[site] = payload
			total += GetInt64FromMap(dr.Value, "total")
		}
	case ResponseDataTypeB:
		for site, dr := range siteResponses {
			data := GetValueFromMap(dr.Value, "data")
			items := GetValueFromMap(data, "items")
			payload := injectSiteToItems(dr.Site, items, sorter)
			payloads[site] = payload
			total += GetInt64FromMap(data, "total")
		}
	case ResponseDataTypeC:
		for site, dr := range siteResponses {
			payload := injectSiteToItems(dr.Site, dr, sorter)
			payloads[site] = payload
		}
	case ResponseDataTypeD:
		for site, dr := range siteResponses {
			items := GetValueFromMap(dr.Value, "autoLearns")
			payload := injectSiteToItems(dr.Site, items, sorter)
			payloads[site] = payload
			total += GetInt64FromMap(dr.Value, "total")
		}
	}

	// 对多个有序列表进行排序， 并取 site，生成 page token ?
	pt := make(PageToken)

	// ASC: 选择小的在前面, DESC: 选择大的在前面
	pick := func(a, b *Payload) (r int) {
		if b == nil {
			return -1
		}

		defer func() {
			if previouspt.IsPrevious(pg) { // 上一页则取最小的
				r = -r
			}
		}()
		defer func() {
			if sorter != nil && sorter.Order == -1 { // 倒序则选择大的
				r = -r
			}
		}()

		if a.Weight > b.Weight {
			return 1
		} else if a.Weight == b.Weight && a.Site.Name > b.Site.Name {
			return 1
		} else if a.Weight == b.Weight && a.Site.Name == b.Site.Name {
			return 0
		}
		return -1
	}

	for i := 0; i < int(pg.Limit); i++ {
		var picked *Payload
		for site, payload := range payloads {
			if _, ok := pt[site]; !ok {
				pt[site] = PageOffset{}
			}

			offset := pt[site]
			if offset.End >= len(payload) {
				continue
			}

			idx := offset.End
			if previouspt.IsPrevious(pg) {
				idx = len(payload) - offset.End - 1
			}

			if pick(payload[idx], picked) == -1 {
				picked = payload[idx]
			}
		}

		if picked != nil {
			offset := pt[picked.Site.Name]
			offset.End++
			pt[picked.Site.Name] = offset

			if previouspt.IsPrevious(pg) {
				items = append([]interface{}{picked.Value}, items...)
			} else {
				items = append(items, picked.Value)
			}
		}
	}

	// 生成对应的响应格式
	if t == ResponseDataTypeA || t == ResponseDataTypeB || t == ResponseDataTypeD {
		res := make(map[string]interface{})
		if previouspt.IsCurrent(pg) {
			res["pageToken"] = previouspt.Encode()
		} else if previouspt.IsNext(pg) {
			res["pageToken"] = previouspt.Add(pt).Encode()
		} else if previouspt.IsPrevious(pg) {
			res["pageToken"] = previouspt.Sub(pt).Encode()
		} else {
			res["pageToken"] = previouspt.Encode()
		}
		response = res
		if t == ResponseDataTypeA {
			res["data"] = items
			res["total"] = total
		} else if t == ResponseDataTypeD {
			res["autoLearns"] = items
			res["total"] = total
		} else {
			res["data"] = map[string]interface{}{
				"total": total,
				"items": items,
			}
		}
	} else if t == ResponseDataTypeC {
		response = items
	}
	return response, nil
}

func (c *client) Proxy(gc *ginlib.GinContext, sites []*siteTypes.Site) (interface{}, error) {
	moduleapi := gc.Query("moduleapi")
	pageToken := gc.Query("pageToken")
	filterSites := gc.QueryArray("site")

	pg := gc.GetPagination()
	pt := DecodePageToken(pageToken)
	endpoint := c.matchEndpoint(moduleapi)

	if endpoint == nil {
		return nil, fmt.Errorf("no matched endpoint: %+v", moduleapi)
	}

	// TODO: 改为并发请求
	siteResponses := make(map[string]*DynamicResponse)
	for i := range sites {
		if len(filterSites) > 0 && !aisUtils.ContainsString(filterSites, sites[i].Name) {
			continue
		}
		req, err := c.BuildSiteRequest(gc, sites[i], moduleapi, pt)
		if err != nil {
			log.WithError(err).Error("failed build site request")
			continue
		}

		raw, resp, err := c.DoRequest(gc, req)
		if err != nil {
			log.WithError(err).Error("failed do site request")
			continue
		}

		siteResponses[sites[i].Name] = &DynamicResponse{
			Value: resp,
			Raw:   raw,
			Site:  sites[i],
		}
	}

	// 注入 site
	// 合并排序分页 处理 pagetoken
	sorter := MapToMemorySorter(endpoint, gc.GetSort())
	return c.MergeSiteResponses(siteResponses, sorter, pt, pg)
}

func max(a, b int64) int64 {
	if a < b {
		return b
	}
	return a
}

func min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func (c *client) parseSitePagination(site *siteTypes.Site, pt PageToken, pg *ginlib.Pagination) *ginlib.Pagination {
	// pt 是上一页的页码偏移
	// pg 是当前页码
	// 根据 pt pg 得出是下一页还是上一页
	// 如果是当前页, 那么 skip = offset.start, limit = size
	// 那么合并各个 site 列表的时候，按照 sort 取 top N
	// 如果是下一页，那么 skip = offset.end + size, limit = size
	// 那么合并各个 site 列表的时候，按照 sort 取 top N
	// 如果是上一页，那么 skip = offset.start - size, limit = size
	// 那么合并各个 site 列表的时候，按照 sort reverse 取 top N
	offset := pt[site.Name]
	if pt.IsCurrent(pg) {
		return &ginlib.Pagination{
			Skip:  int64(offset.Start),
			Limit: pg.GetPageSize(),
		}
	} else if pt.IsPrevious(pg) { // 上一页
		return &ginlib.Pagination{
			Skip:  max(int64(offset.Start)-pg.Limit, 0),
			Limit: min(pg.GetPageSize(), int64(offset.Start)),
		}
	} else if pt.IsNext(pg) { // 下一页
		return &ginlib.Pagination{
			Skip:  int64(offset.End),
			Limit: pg.GetPageSize(),
		}
	}
	log.Error("parse site pagination, bad pageToken!")
	return &ginlib.Pagination{
		Skip:  0,
		Limit: pg.GetPageSize(),
	}
}

func (c *client) matchEndpoint(apipath string) *Endpoint {
	for _, endpoint := range EndpointAPIPrefixes {
		if strings.HasPrefix(apipath, endpoint.APIPrefix) {
			return endpoint
		}
	}
	return nil
}

func (c *client) buildSiteURI(site *siteTypes.Site, endpoint *Endpoint, apipath string, inCluster, useAdminDomain bool) string {
	domain := site.Domain
	if useAdminDomain {
		domain = site.AdminDomain
	}

	if strings.HasPrefix(apipath, endpoint.APIPrefix) {
		if inCluster {
			return fmt.Sprintf("%s%s%s", endpoint.URI, endpoint.RewritePrefix, strings.TrimPrefix(apipath, endpoint.APIPrefix))
		}
		if strings.Contains(domain, "http://") || strings.Contains(domain, "https://") {
			return fmt.Sprintf("%s%s", domain, apipath)
		}
		return fmt.Sprintf("https://%s%s", domain, apipath)
	}
	return ""
}

func (c *client) SitesClusterAPU(gc *ginlib.GinContext, sites []*siteTypes.Site) (*SitesClusterAPU, error) {
	moduleapi := "/aapi/resourcesense.ais.io/api/v1/quotas/cluster/apu"
	_ = c.matchEndpoint(moduleapi)

	type responseT struct {
		Data []*aisTypes.APUConfig
	}

	sc := &SitesClusterAPU{
		Distribution: make(map[string][]*aisTypes.APUConfig),
	}
	for i := range sites {
		req, err := c.BuildSiteRequest(gc, sites[i], moduleapi, nil)
		if err != nil {
			log.WithError(err).Error("failed build site request")
			continue
		}

		raw, _, err := c.DoRequest(gc, req)
		if err != nil {
			log.WithError(err).Error("failed do site request")
			continue
		}

		var res responseT
		if err := json.Unmarshal(raw, &res); err != nil {
			log.WithError(err).Error("failed to Unmarshal")
			continue
		}
		sc.Distribution[sites[i].Name] = res.Data
	}

	return sc, nil
}

func (c *client) SitesClusterQuota(gc *ginlib.GinContext, sites []*siteTypes.Site) (*SitesClusterQuota, error) {
	// 1. 调用所有 site 的 quotas/cluster api
	// 2. 合并成响应返回
	// TODO: 改为并发请求
	moduleapi := "/aapi/resourcesense.ais.io/api/v1/quotas/cluster"
	_ = c.matchEndpoint(moduleapi)

	siteResponses := make(map[string]*DynamicResponse)

	for i := range sites {
		req, err := c.BuildSiteRequest(gc, sites[i], moduleapi, nil)
		if err != nil {
			log.WithError(err).Error("failed build site request")
			continue
		}

		raw, resp, err := c.DoRequest(gc, req)
		if err != nil {
			log.WithError(err).Error("failed do site request")
			continue
		}

		siteResponses[sites[i].Name] = &DynamicResponse{
			Value: resp,
			Raw:   raw,
			Site:  sites[i],
		}
	}

	// 注入 site
	// 合并排序分页 处理 pagetoken
	return c.MergeSitesClusterQuotaResponses(siteResponses)
}

func (c *client) MergeSitesClusterQuotaResponses(siteResponses map[string]*DynamicResponse) (*SitesClusterQuota, error) {
	overview := &quotaTypes.ClusterQuota{}
	scq := SitesClusterQuota{
		Overview:     overview,
		Distribution: make(map[string]*quotaTypes.ClusterQuota),
	}

	type responseT struct {
		Data *quotaTypes.ClusterQuota
	}

	for site, dr := range siteResponses {
		var res responseT
		if err := json.Unmarshal(dr.Raw, &res); err != nil {
			log.WithError(err).Errorf("fuck unmarshal json")
			continue
		}
		res.Data.FromData()
		overview.Add(res.Data)
		scq.Distribution[site] = res.Data
	}
	overview.ToData()
	return &scq, nil
}

func (c *client) SitesTenantQuota(gc *ginlib.GinContext, sites []*siteTypes.Site) (*SitesClusterQuota, error) {
	// 1. 调用所有 site 的 quotas/cluster api
	// 2. 合并成响应返回
	// TODO: 改为并发请求
	moduleapi := gc.Query("moduleapi")
	_ = c.matchEndpoint(moduleapi)

	siteResponses := make(map[string]*DynamicResponse)

	for i := range sites {
		req, err := c.BuildSiteRequest(gc, sites[i], moduleapi, nil)
		if err != nil {
			log.WithError(err).Error("fuck build site request")
			continue
		}

		raw, resp, err := c.DoRequest(gc, req)
		if err != nil {
			log.WithError(err).Error("fuck build site request")
			continue
		}

		siteResponses[sites[i].Name] = &DynamicResponse{
			Value: resp,
			Raw:   raw,
			Site:  sites[i],
		}
	}
	return c.MergeSitesTenantQuotaResponses(siteResponses)
}

func (c *client) MergeSitesTenantQuotaResponses(siteResponses map[string]*DynamicResponse) (*SitesClusterQuota, error) {
	overview := &quotaTypes.ClusterQuota{}
	scq := SitesClusterQuota{
		Overview:     overview,
		Distribution: make(map[string]*quotaTypes.ClusterQuota),
	}

	type responseT struct {
		Data *quotaTypes.TenantDetail
	}

	for site, dr := range siteResponses {
		var res responseT
		if err := json.Unmarshal(dr.Raw, &res); err != nil {
			log.WithError(err).Errorf("fuck unmarshal json")
			continue
		}
		res.Data.FromData()
		cq := &quotaTypes.ClusterQuota{
			TotalQuota:   res.Data.TotalQuota,
			ChargedQuota: res.Data.ChargedQuota,
			Used:         res.Data.Used,
		}
		cq.ToData()
		overview.TotalQuota = overview.TotalQuota.Add(res.Data.TotalQuota)
		overview.ChargedQuota = overview.ChargedQuota.Add(res.Data.ChargedQuota)
		overview.Used = overview.ChargedQuota.Add(res.Data.Used)
		scq.Distribution[site] = cq
	}
	overview.ToData()
	return &scq, nil
}

func (c *client) ReportStatus(gc *ginlib.GinContext, self, center *siteTypes.Site) error {
	// 1. 调用所有 site 的 quotas/cluster api
	// 2. 合并成响应返回
	// TODO: 改为并发请求
	moduleapi := "/aapi/resourcesense.ais.io/api/v1/sites/communication/reportstatus"
	endpoint := c.matchEndpoint(moduleapi)
	if endpoint == nil {
		return fmt.Errorf("no match endpoint: %s", moduleapi)
	}
	uri := c.buildSiteURI(center, endpoint, moduleapi, false, false)

	bodyBytes, _ := json.Marshal(&ReportPayload{
		SiteInfo:  self,
		Timestamp: time.Now().Unix(),
	})
	req, err := http.NewRequest("POST", uri, bytes.NewReader(bodyBytes))
	if err != nil {
		log.WithError(err).Error("failed to release by user request")
		return err
	}

	req.Header.Set(authTypes.AISAuthHeader, gc.GetAuthToken()) // TODO: 确认是否有问题?
	req.Header.Set("Content-Type", "application/json")
	_, _, err = c.DoRequest(gc, req)
	if err != nil {
		log.WithError(err).Error("fuck report site status")
		return err
	}
	return nil
}
