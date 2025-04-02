package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/25x8/ya-practicum-6-sprint/internal/models"
)

type AccrualService struct {
	baseURL    string
	httpClient *http.Client
}

func NewAccrualService(baseURL string) *AccrualService {
	return &AccrualService{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (s *AccrualService) GetOrderAccrual(ctx context.Context, orderNumber string) (*models.AccrualResponse, error) {
	url := fmt.Sprintf("%s/api/orders/%s", s.baseURL, orderNumber)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		retryAfter := resp.Header.Get("Retry-After")
		if retryAfter != "" {
			seconds, err := strconv.Atoi(retryAfter)
			if err == nil {
				return nil, fmt.Errorf("rate limited, retry after %d seconds", seconds)
			}
		}
		return nil, fmt.Errorf("rate limited")
	}

	if resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("accrual service returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var accrualResp models.AccrualResponse
	if err := json.Unmarshal(body, &accrualResp); err != nil {
		return nil, err
	}

	return &accrualResp, nil
}
