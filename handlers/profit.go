package handlers

import (
	"encoding/json"
	"net/http"
	"route-margin-engine/services"
)

// ProfitRequest defines the incoming payload structure from UI
type ProfitRequest struct {
	Origin          string  `json:"origin"`            // e.g. "Lagos, Nigeria"
	Destination     string  `json:"destination"`       // e.g. "Kano, Nigeria"
	QuotedFreight   float64 `json:"quoted_freight"`    // Price charged to shipper (NGN)
	FuelEfficiency  float64 `json:"fuel_efficiency"`   // km per Liter (e.g. 2.2 for 30-ton trailer)
	DieselPricePerL float64 `json:"diesel_price_per_l"` // NGN per liter (defaults to market benchmark if 0)
	DriverAllowance float64 `json:"driver_allowance"`  // Daily/Trip allowance for driver & conductor (NGN)
	TruckType       string  `json:"truck_type"`        // e.g., "30-Ton Flatbed", "15-Ton Rigid"
}

// ProfitResponse defines the complete route analysis output
type ProfitResponse struct {
	DistanceKm           float64                        `json:"distance_km"`
	EstimatedDuration    string                         `json:"estimated_duration"`
	FuelCostNaira        float64                        `json:"fuel_cost_naira"`
	BaseWearCostNaira    float64                        `json:"base_wear_cost_naira"`
	TotalOperationalCost float64                        `json:"total_operational_cost"`
	NetProfitNaira       float64                        `json:"net_profit_naira"`
	ProfitMarginPct      float64                        `json:"profit_margin_pct"`
	Recommendation       string                         `json:"recommendation"` // "HIGHLY PROFITABLE", "MARGINAL RISK", "UNPROFITABLE - REJECT"
	AIRiskAssessment     *services.GeminiRiskAssessment `json:"ai_risk_assessment"`
}

func HandleProfitCalculation(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"Only POST requests are permitted"}`, http.StatusMethodNotAllowed)
		return
	}

	var req ProfitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"Invalid payload structure"}`, http.StatusBadRequest)
		return
	}

	if req.Origin == "" || req.Destination == "" || req.QuotedFreight <= 0 {
		http.Error(w, `{"error":"Origin, Destination, and valid QuotedFreight are required"}`, http.StatusBadRequest)
		return
	}

	// Fallback for Fuel Efficiency if unprovided (2.5 km/L standard for 20-30 ton trucks)
	if req.FuelEfficiency <= 0 {
		req.FuelEfficiency = 2.5
	}

	// Fallback for Diesel Price (~₦1,750/L market benchmark)
	if req.DieselPricePerL <= 0 {
		req.DieselPricePerL = 1750.0
	}

	// Fallback for Truck Type if missing
	if req.TruckType == "" {
		req.TruckType = "30-Ton Flatbed"
	}

	// 1. Fetch deterministic route metrics (Distance & Duration) from Google Maps service
	routeData, err := services.FetchRouteData(req.Origin, req.Destination)
	if err != nil {
		http.Error(w, `{"error":"Failed to retrieve route parameters from Maps service"}`, http.StatusInternalServerError)
		return
	}

	// 2. Query Gemini 1.5 Flash for corridor degradation, localized LGA checkpoints, informal levies & backhaul risk
	aiAssessment, err := services.EvaluateCorridorRisk(req.Origin, req.Destination, req.TruckType, routeData.DistanceKm)
	if err != nil {
		// Non-blocking fallback if AI service fails or times out
		aiAssessment = &services.GeminiRiskAssessment{
			RoadQualityRisk:     "Medium",
			BackhaulRisk:        "Medium",
			EstimatedTollsNaira: 25000.0,
			KeyLGACheckpoints: []string{
				"State Boundary / Local Council Heavy Duty Stickers",
				"Agricultural / Produce Inspection Control Points",
			},
			Summary: "Standard Nigerian freight corridor. Expect localized state stickers and revenue checkpoints along major transit towns.",
		}
	}

	// 3. Compute Deterministic Financials
	fuelConsumedLiters := routeData.DistanceKm / req.FuelEfficiency
	totalFuelCost := fuelConsumedLiters * req.DieselPricePerL

	// Base Asset Depreciation & Wear (₦200 per km for heavy chassis/tires on Nigerian road surfaces)
	baseWearCost := routeData.DistanceKm * 200.0

	// Dynamic Operational Cost calculation factoring AI Levy & Toll estimates
	totalOpsCost := totalFuelCost + baseWearCost + req.DriverAllowance + aiAssessment.EstimatedTollsNaira
	netProfit := req.QuotedFreight - totalOpsCost
	marginPct := (netProfit / req.QuotedFreight) * 100.0

	// 4. Decision Engine Threshold Logic
	var recommendation string
	switch {
	case marginPct >= 25.0 && aiAssessment.BackhaulRisk != "High":
		recommendation = "HIGHLY PROFITABLE (Safe operational buffer)"
	case marginPct >= 12.0:
		recommendation = "MARGINAL RISK (Tight margins; ensure quick offloading)"
	default:
		recommendation = "UNPROFITABLE - REJECT (Risk of operational loss)"
	}

	response := ProfitResponse{
		DistanceKm:           routeData.DistanceKm,
		EstimatedDuration:    routeData.Duration,
		FuelCostNaira:        totalFuelCost,
		BaseWearCostNaira:    baseWearCost,
		TotalOperationalCost: totalOpsCost,
		NetProfitNaira:       netProfit,
		ProfitMarginPct:      marginPct,
		Recommendation:       recommendation,
		AIRiskAssessment:     aiAssessment,
	}

	json.NewEncoder(w).Encode(response)
}