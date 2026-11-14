// Package corekit is the HTTP client payment-service uses to talk to core-service.
package corekit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

var (
	ErrInsufficientFunds = errors.New("insufficient funds")
	ErrWalletNotFound    = errors.New("wallet not found")
)

type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

func New(baseURL, token string) *Client {
	return &Client{BaseURL: baseURL, Token: token, HTTP: &http.Client{Timeout: 5 * time.Second}}
}

type DebitRequest struct {
	Reference   string  `json:"reference"`
	UserID      string  `json:"userID"`
	MerchantID  string  `json:"merchantID"`
	Description string  `json:"description"`
	Amount      float64 `json:"amount"`
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) (int, error) {
	var rdr *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, rdr)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Token", c.Token)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if out != nil && resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return resp.StatusCode, err
		}
	}
	return resp.StatusCode, nil
}

// Balance returns the user's current wallet balance.
func (c *Client) Balance(ctx context.Context, userID string) (float64, error) {
	var out struct {
		Balance float64 `json:"balance"`
	}
	code, err := c.do(ctx, http.MethodGet, "/internal/wallets/"+userID+"/balance", nil, &out)
	if err != nil {
		return 0, err
	}
	switch code {
	case http.StatusOK:
		return out.Balance, nil
	case http.StatusNotFound:
		return 0, ErrWalletNotFound
	default:
		return 0, fmt.Errorf("core-service returned %d", code)
	}
}

// Debit deducts the wallet; safe to retry because core-service dedupes on Reference.
func (c *Client) Debit(ctx context.Context, r DebitRequest) (float64, error) {
	var out struct {
		NewBalance float64 `json:"newBalance"`
	}
	code, err := c.do(ctx, http.MethodPost, "/internal/wallets/debit", r, &out)
	if err != nil {
		return 0, err
	}
	switch code {
	case http.StatusOK:
		return out.NewBalance, nil
	case http.StatusBadRequest:
		return 0, ErrInsufficientFunds
	default:
		return 0, fmt.Errorf("core-service returned %d", code)
	}
}
