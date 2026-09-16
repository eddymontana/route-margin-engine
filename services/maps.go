package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

type RouteResult struct {
	DistanceKm float64 `json:"distance_km"`
	Duration   string  `json:"duration"`
}

type routesRequest struct {
	Origin      locationSpec `json:"origin"`
	Destination locationSpec `json:"destination"`
	TravelMode  string       `json:"travelMode"`
}

type locationSpec struct {
	Address string `json:"address"`
}

type routesResponse struct {
	Routes []struct {
		DistanceMeters int    `json:"distanceMeters"`
		Duration       string `json:"duration"`
	} `json:"routes"`
}

// FetchRouteData calls Google Maps Routes API v2
func FetchRouteData(originStr, destStr string) (*RouteResult, error) {
	apiKey := os.Getenv("MAPS_API_KEY")

	// Dev Fallback Mock if API Key is not configured
	if apiKey == "" || apiKey == "YOUR_GOOGLE_MAPS_API_KEY" {
		return &RouteResult{
			DistanceKm: 650.0,
			Duration:   "10 hours 30 mins",
		}, nil
	}

	url := "https://routes.googleapis.com/directions/v2:computeRoutes"

	reqBody := routesRequest{
		Origin:      locationSpec{Address: originStr},
		Destination: locationSpec{Address: destStr},
		TravelMode:  "DRIVE",
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to encode route request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Goog-Api-Key", apiKey)
	req.Header.Set("X-Goog-FieldMask", "routes.distanceMeters,routes.duration")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("routes api call failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("routes api returned non-200 status: %d", resp.StatusCode)
	}

	var parsedResp routesResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsedResp); err != nil {
		return nil, fmt.Errorf("failed to decode routes response: %w", err)
	}

	if len(parsedResp.Routes) == 0 {
		return nil, fmt.Errorf("no routes found between %s and %s", originStr, destStr)
	}

	route := parsedResp.Routes[0]
	distKm := float64(route.DistanceMeters) / 1000.0

	return &RouteResult{
		DistanceKm: distKm,
		Duration:   route.Duration,
	}, nil
}