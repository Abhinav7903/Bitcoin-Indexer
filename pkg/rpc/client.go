package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

const (
	defaultTimeout = 30 * time.Second
	maxRetries     = 3
)

type Client struct {
	url      string
	user     string
	password string
	client   *http.Client
}

func NewClient(url, user, password string) *Client {
	return &Client{
		url:      url,
		user:     user,
		password: password,
		client: &http.Client{
			Timeout: defaultTimeout,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 100,
				IdleConnTimeout:     90 * time.Second,
				DisableCompression:  false,
			},
		},
	}
}

func (c *Client) call(method string, params []interface{}, res interface{}) error {
	payload := map[string]interface{}{
		"jsonrpc": "1.0",
		"id":      "btc-indexer",
		"method":  method,
		"params":  params,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)

		req, err := http.NewRequestWithContext(
			ctx,
			http.MethodPost,
			c.url,
			bytes.NewBuffer(body),
		)
		if err != nil {
			cancel()
			return err
		}

		req.SetBasicAuth(c.user, c.password)
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.client.Do(req)

		if err != nil {
			cancel()

			if isRetryable(err) {
				lastErr = err
				time.Sleep(time.Duration(attempt+1) * time.Second)
				continue
			}

			return err
		}

		func() {
			defer resp.Body.Close()

			if resp.StatusCode >= 500 {
				b, _ := io.ReadAll(resp.Body)
				lastErr = fmt.Errorf(
					"rpc %s failed: %s - %s",
					method,
					resp.Status,
					string(b),
				)
				return
			}

			if resp.StatusCode != http.StatusOK {
				b, _ := io.ReadAll(resp.Body)
				lastErr = fmt.Errorf(
					"rpc %s failed: %s - %s",
					method,
					resp.Status,
					string(b),
				)
				return
			}

			decoder := json.NewDecoder(resp.Body)
			decoder.UseNumber()

			lastErr = decoder.Decode(res)
		}()

		cancel()

		if lastErr == nil {
			return nil
		}

		time.Sleep(time.Duration(attempt+1) * time.Second)
	}

	return lastErr
}

func isRetryable(err error) bool {
	if netErr, ok := err.(net.Error); ok {
		return netErr.Timeout() || netErr.Temporary()
	}
	return true
}

func (c *Client) GetBlockCount() (int32, error) {
	var response struct {
		Result json.Number `json:"result"`
		Error  interface{} `json:"error"`
	}

	if err := c.call("getblockcount", []interface{}{}, &response); err != nil {
		return 0, err
	}

	if response.Error != nil {
		return 0, fmt.Errorf("rpc getblockcount error: %v", response.Error)
	}

	count, err := response.Result.Int64()
	return int32(count), err
}

func (c *Client) GetBlockHash(height int32) (string, error) {
	var response struct {
		Result string      `json:"result"`
		Error  interface{} `json:"error"`
	}

	if err := c.call("getblockhash", []interface{}{height}, &response); err != nil {
		return "", err
	}

	if response.Error != nil {
		return "", fmt.Errorf("rpc getblockhash error: %v", response.Error)
	}

	return response.Result, nil
}

func (c *Client) GetBlockVerbose(hash string) (map[string]interface{}, error) {
	var response struct {
		Result map[string]interface{} `json:"result"`
		Error  interface{}            `json:"error"`
	}

	if err := c.call("getblock", []interface{}{hash, 2}, &response); err != nil {
		return nil, err
	}

	if response.Error != nil {
		return nil, fmt.Errorf("rpc getblock error: %v", response.Error)
	}

	return response.Result, nil
}
