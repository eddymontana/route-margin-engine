package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// GeminiRiskAssessment holds the AI evaluation of route risk and LGA checkpoints
type GeminiRiskAssessment struct {
	RoadQualityRisk     string   `json:"road_quality_risk"`     // Low, Medium, High, Severe
	BackhaulRisk        string   `json:"backhaul_risk"`         // Low, Medium, High
	EstimatedTollsNaira float64  `json:"estimated_tolls_naira"`  // Estimated informal/state/LGA levies
	KeyLGACheckpoints   []string `json:"key_lga_checkpoints"`   // Specific LGA boundaries, produce/sticker ticket zones
	Summary             string   `json:"summary"`               // Executive corridor summary
}

// Internal structures for Gemini REST API payload
type geminiRequest struct {
	Contents []geminiContent `json:"contents"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

// EvaluateCorridorRisk queries Gemini 1.5 Flash for transit risks, LGA revenue bottlenecks, and estimated levies.
func EvaluateCorridorRisk(origin, destination, truckType string, distanceKm float64) (*GeminiRiskAssessment, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY environment variable is not set")
	}

	prompt := fmt.Sprintf("You are an expert Nigerian freight corridor analyst. Analyze the heavy freight transit route from \"%s\" to \"%s\" for a \"%s\" carrying commercial goods over ~%.1f km.\n\n"+
		"Provide a strict, high-precision structured analysis focusing on:\n"+
		"1. Key Local Government Areas (LGAs) and State Boundaries crossed where informal or state-level levies (e.g., Produce tickets, Local Council Stickers, Heavy Duty Vehicle levies, Forestry fees) are actively enforced.\n"+
		"2. Major road quality degradation, security bottlenecks, and known delay zones.\n"+
		"3. Total estimated state/LGA informal levies and stickers in NGN (numeric value).\n"+
		"4. Backhaul cargo availability risk at the destination point.\n\n"+
		"CRITICAL INSTRUCTION: Return ONLY a valid JSON object matching this exact schema. Do NOT include markdown code blocks, commentary, or extra text before/after:\n\n"+
		"{\n"+
		"  \"road_quality_risk\": \"Medium\",\n"+
		"  \"backhaul_risk\": \"Low\",\n"+
		"  \"estimated_tolls_naira\": 35000,\n"+
		"  \"key_lga_checkpoints\": [\n"+
		"    \"Ijebu-Ode LGA (Ogun State) - Produce & Sticker Enforcement\",\n"+
		"    \"Lokoja LGA (Kogi State) - Interstate Transit Tolls & Heavy Axle Levy\"\n"+
		"  ],\n"+
		"  \"summary\": \"Corridor features moderate road deterioration through specific bypasses with multiple LGA collection points in transit states.\"\n"+
		"}", origin, destination, truckType, distanceKm)

	reqBody := geminiRequest{
		Contents: []geminiContent{
			{
				Parts: []geminiPart{
					{Text: prompt},
				},
			},
		},
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Gemini request: %w", err)
	}

	apiURL := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-1.5-flash:generateContent?key=%s", apiKey)

	client := &http.Client{Timeout: 12 * time.Second}
	resp, err := client.Post(apiURL, "application/json", bytes.NewBuffer(jsonBytes))
	if err != nil {
		return nil, fmt.Errorf("gemini HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gemini API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var geminiResp geminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&geminiResp); err != nil {
		return nil, fmt.Errorf("failed to decode Gemini response: %w", err)
	}

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("empty response received from Gemini model")
	}

	rawText := geminiResp.Candidates[0].Content.Parts[0].Text

	// Clean Markdown code block markers if returned by the LLM
	cleanedText := cleanJSONString(rawText)

	var assessment GeminiRiskAssessment
	if err := json.Unmarshal([]byte(cleanedText), &assessment); err != nil {
		return nil, fmt.Errorf("failed to unmarshal Gemini risk assessment JSON: %w (raw: %s)", err, rawText)
	}

	return &assessment, nil
}

// Helper function to strip markdown tags (e.g. ```json ... ```)
func cleanJSONString(input string) string {
	b := []byte(input)
	b = bytes.TrimSpace(b)
	b = bytes.TrimPrefix(b, []byte("```json"))
	b = bytes.TrimPrefix(b, []byte("```"))
	b = bytes.TrimSuffix(b, []byte("```"))
	return string(bytes.TrimSpace(b))
}