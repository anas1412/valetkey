// Package promql is a minimal client for the Prometheus HTTP query API, used to
// show usage history that outlives the pod.
package promql

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	BaseURL string
	HTTP    *http.Client
}

func New(baseURL string) *Client {
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), HTTP: &http.Client{Timeout: 10 * time.Second}}
}

// Sample is one labelled value of an instant query.
type Sample struct {
	Labels map[string]string
	Value  float64
}

// Point is one value of a range query.
type Point struct {
	Time  time.Time
	Value float64
}

// Series is one labelled series of a range query.
type Series struct {
	Labels map[string]string
	Points []Point
}

// Query runs an instant query at time t.
func (c *Client) Query(ctx context.Context, q string, t time.Time) ([]Sample, error) {
	var res []struct {
		Metric map[string]string `json:"metric"`
		Value  [2]any            `json:"value"`
	}
	if err := c.get(ctx, "/api/v1/query", url.Values{"query": {q}, "time": {unix(t)}}, "vector", &res); err != nil {
		return nil, err
	}
	out := make([]Sample, 0, len(res))
	for _, r := range res {
		v, err := value(r.Value[1])
		if err != nil {
			return nil, err
		}
		out = append(out, Sample{Labels: r.Metric, Value: v})
	}
	return out, nil
}

// QueryRange runs a range query from start to end at the given step.
func (c *Client) QueryRange(ctx context.Context, q string, start, end time.Time, step time.Duration) ([]Series, error) {
	var res []struct {
		Metric map[string]string `json:"metric"`
		Values [][2]any          `json:"values"`
	}
	params := url.Values{"query": {q}, "start": {unix(start)}, "end": {unix(end)}, "step": {strconv.Itoa(int(step.Seconds()))}}
	if err := c.get(ctx, "/api/v1/query_range", params, "matrix", &res); err != nil {
		return nil, err
	}
	out := make([]Series, 0, len(res))
	for _, r := range res {
		s := Series{Labels: r.Metric}
		for _, pair := range r.Values {
			ts, ok := pair[0].(float64)
			if !ok {
				return nil, fmt.Errorf("unexpected timestamp %v", pair[0])
			}
			v, err := value(pair[1])
			if err != nil {
				return nil, err
			}
			s.Points = append(s.Points, Point{Time: time.Unix(int64(ts), 0), Value: v})
		}
		out = append(out, s)
	}
	return out, nil
}

func (c *Client) get(ctx context.Context, path string, params url.Values, wantType string, into any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path+"?"+params.Encode(), nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var body struct {
		Status string `json:"status"`
		Error  string `json:"error"`
		Data   struct {
			ResultType string          `json:"resultType"`
			Result     json.RawMessage `json:"result"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return fmt.Errorf("prometheus: HTTP %d, unreadable answer", resp.StatusCode)
	}
	if body.Status != "success" {
		return fmt.Errorf("prometheus: %s", body.Error)
	}
	if body.Data.ResultType != wantType {
		return fmt.Errorf("prometheus: expected %s, got %s", wantType, body.Data.ResultType)
	}
	return json.Unmarshal(body.Data.Result, into)
}

// value parses Prometheus's string-encoded sample values ("1.5", "NaN", "+Inf").
func value(v any) (float64, error) {
	s, ok := v.(string)
	if !ok {
		return 0, fmt.Errorf("unexpected value %v", v)
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f != f { // NaN counts as no data
		return 0, nil
	}
	return f, nil
}

func unix(t time.Time) string { return strconv.FormatInt(t.Unix(), 10) }
