package relay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"

	"github.com/anas1412/valetkey/internal/types"
)

const listTimeout = 10 * time.Second

// ErrKeyRejected means the provider answered 401 or 403: the API key is wrong or revoked.
var ErrKeyRejected = errors.New("provider rejected the API key")

// RejectedReason turns a ListModels error into a key verdict: only a 401/403 answer
// means the key is bad; anything else (timeout, no models endpoint) says nothing.
func RejectedReason(err error) string {
	if errors.Is(err, ErrKeyRejected) {
		return err.Error()
	}
	return ""
}

// ModelList is a provider's model list call: what the dashboard shows when picking models.
type ModelList struct {
	Models    []string `json:"models"`
	Status    int      `json:"status,omitempty"`
	LatencyMs int64    `json:"latencyMs"`
}

// ListModels calls the provider's GET /v1/models with its real key. No prompt is sent.
func ListModels(ctx context.Context, ch *types.Provider, apiKey string) (ModelList, error) {
	var res ModelList
	url, err := JoinURL(ch.Spec.BaseURL, "/v1/models")
	if err != nil {
		return res, err
	}
	ctx, cancel := context.WithTimeout(ctx, listTimeout)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if ch.Spec.Type == "anthropic" {
		req.Header.Set("X-Api-Key", apiKey)
		req.Header.Set("Anthropic-Version", "2023-06-01")
	} else {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	start := time.Now()
	resp, err := NewTransports().Client(ch).Do(req)
	res.LatencyMs = time.Since(start).Milliseconds()
	if err != nil {
		return res, err
	}
	defer resp.Body.Close()
	res.Status = resp.StatusCode
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return res, fmt.Errorf("%w (HTTP %d)", ErrKeyRejected, resp.StatusCode)
	}
	if resp.StatusCode >= 300 {
		return res, fmt.Errorf("provider answered HTTP %d", resp.StatusCode)
	}
	var body struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&body); err != nil {
		return res, fmt.Errorf("provider's model list is not in the expected format")
	}
	for _, m := range body.Data {
		if m.ID != "" {
			res.Models = append(res.Models, m.ID)
		}
	}
	sort.Strings(res.Models)
	return res, nil
}
