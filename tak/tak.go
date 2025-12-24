package tak

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Telemetry matches the JSON payload exactly (note the json tags with spaces and slashes).
type Telemetry struct {
	SerialNumber float64   `json:"serial number"`
	DateTime     time.Time `json:"date/time"` // Marshals RFC3339 by default
	Latitude     float64   `json:"Latitude"`
	Longitude    float64   `json:"Longitude"`
	Event        int       `json:"Event"`
	SolarPower   float64   `json:"solar power"` // Strings in your sample payload
	Speed        float64   `json:"Speed"`       // Strings in your sample payload
	Heading      int       `json:"Heading"`
}

// PostTelemetry sends the JSON to the endpoint, optionally skipping TLS verification
// (equivalent to curl --insecure). It returns the response body bytes.
func PostTelemetryTAK(ctx context.Context, endpoint string, tlsConfig *tls.Config, data Telemetry, insecureTLS bool) ([]byte, error) {
	// TLS transport that can skip certificate verification when requested.

	// TODO LEAVE OUT tlsConfig.InsecureSkipVerify = insecureTLS

	tr := &http.Transport{
		TLSClientConfig: tlsConfig,
	}

	client := &http.Client{
		Transport: tr,
		Timeout:   15 * time.Second,
	}

	// Marshal to JSON.
	body, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("marshal telemetry: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Connection", "close")
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("post: %w", err)
	}

	defer func() {
		err := resp.Body.Close()
		if err != nil {
			return
		}
	}()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return respBody, fmt.Errorf("non-2xx status: %s", resp.Status)
	}
	return respBody, nil
}
