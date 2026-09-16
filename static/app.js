// Environment URL configuration with local fallback
const API_BASE_URL = window.location.hostname === "localhost" || window.location.hostname === "127.0.0.1"
    ? "http://localhost:8080"
    : "https://route-margin-engine-1097034071715.europe-west1.run.app";

document.addEventListener("DOMContentLoaded", () => {
    const calcForm = document.getElementById("route-form");
    if (!calcForm) return;

    calcForm.addEventListener("submit", async (e) => {
        e.preventDefault();

        const submitBtn = document.getElementById("submit-btn");
        const resultsDiv = document.getElementById("results-output");
        
        // Indicate active state
        if (submitBtn) submitBtn.disabled = true;
        if (resultsDiv) resultsDiv.innerHTML = "<p class='loading'>Calculating route profitability via Gemini AI...</p>";

        // Extract form values
        const payload = {
            origin: document.getElementById("origin").value.trim(),
            destination: document.getElementById("destination").value.trim(),
            quoted_freight: parseFloat(document.getElementById("quoted_freight").value),
            fuel_efficiency: parseFloat(document.getElementById("fuel_efficiency").value),
            diesel_price_per_l: parseFloat(document.getElementById("diesel_price_per_l").value),
            driver_allowance: parseFloat(document.getElementById("driver_allowance").value),
            truck_type: document.getElementById("truck_type").value
        };

        try {
            const response = await fetch(`${API_BASE_URL}/api/profit`, {
                method: "POST",
                headers: {
                    "Content-Type": "application/json"
                },
                body: JSON.stringify(payload)
            });

            if (response.status === 429) {
                throw new Error("Rate limit exceeded. Please wait a moment before trying again.");
            }

            if (!response.ok) {
                const errData = await response.json();
                throw new Error(errData.error || `Server responded with status ${response.status}`);
            }

            const data = await response.json();
            renderResults(data);

        } catch (err) {
            if (resultsDiv) {
                resultsDiv.innerHTML = `<div class="error-box"><strong>Error:</strong> ${err.message}</div>`;
            }
        } finally {
            if (submitBtn) submitBtn.disabled = false;
        }
    });
});

function renderResults(data) {
    const resultsDiv = document.getElementById("results-output");
    if (!resultsDiv) return;

    const formattedNet = new Intl.NumberFormat('en-NG', { style: 'currency', currency: 'NGN' }).format(data.net_profit_naira);
    const formattedCost = new Intl.NumberFormat('en-NG', { style: 'currency', currency: 'NGN' }).format(data.total_operational_cost);

    resultsDiv.innerHTML = `
        <div class="metrics-grid">
            <div class="metric-card">
                <h3>Margin</h3>
                <p class="highlight">${data.profit_margin_pct.toFixed(1)}%</p>
            </div>
            <div class="metric-card">
                <h3>Net Profit</h3>
                <p class="highlight">${formattedNet}</p>
            </div>
            <div class="metric-card">
                <h3>Total Cost</h3>
                <p>${formattedCost}</p>
            </div>
            <div class="metric-card">
                <h3>Distance</h3>
                <p>${data.distance_km} km (${data.estimated_duration})</p>
            </div>
        </div>

        <div class="ai-assessment">
            <h4>AI Risk Assessment</h4>
            <p><strong>Recommendation:</strong> ${data.recommendation}</p>
            <p><strong>Road Risk:</strong> ${data.ai_risk_assessment.road_quality_risk} | <strong>Backhaul Risk:</strong> ${data.ai_risk_assessment.backhaul_risk}</p>
            <p><strong>Summary:</strong> ${data.ai_risk_assessment.summary}</p>
        </div>
    `;
}