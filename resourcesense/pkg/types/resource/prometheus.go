package resource

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

type Result struct {
	Metric any     `json:"metric"`
	Values [][]any `json:"values"`
}

type Data struct {
	ResultType string   `json:"resultType"`
	Result     []Result `json:"result"`
}

type PromResponse struct {
	Status string `json:"status"`
	Data   Data   `json:"data"`
}

func GetPromResponse(promethuesServer, query string, start, end, step int64) (*PromResponse, error) {
	var pr PromResponse
	httpClient := http.Client{
		Timeout: 10 * time.Second,
	}

	params := url.Values{}
	purl, err := url.Parse(fmt.Sprintf("%s/api/v1/query_range", promethuesServer))
	if err != nil {
		return nil, err
	}

	params.Set("end", fmt.Sprintf("%d", end))
	params.Set("start", fmt.Sprintf("%d", start))
	params.Set("query", query)
	params.Set("step", fmt.Sprintf("%d", step))

	purl.RawQuery = params.Encode()
	resp, err := httpClient.Get(purl.String())
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(body, &pr); err != nil {
		return nil, err
	}

	return &pr, nil
}
