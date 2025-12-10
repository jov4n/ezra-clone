# Test LiteLLM directly with different model formats

$headers = @{
    "Content-Type" = "application/json"
}

# Test different model formats
$models = @(
    "openrouter/google/gemini-2.5-flash",
    "google/gemini-2.5-flash"
)

foreach ($model in $models) {
    Write-Host "`nTesting LiteLLM with model: $model" -ForegroundColor Yellow
    
    $body = @{
        model = $model
        messages = @(
            @{
                role = "user"
                content = "Hello"
            }
        )
    } | ConvertTo-Json

    try {
        $response = Invoke-RestMethod -Uri "http://localhost:4000/v1/chat/completions" -Method Post -Headers $headers -Body $body -ErrorAction Stop
        Write-Host "SUCCESS: LiteLLM accepts $model" -ForegroundColor Green
        Write-Host "Response: $($response.choices[0].message.content)" -ForegroundColor Cyan
        break
    } catch {
        $errorMsg = $_.Exception.Message
        if ($_.ErrorDetails.Message) {
            $errorMsg = $_.ErrorDetails.Message
        }
        Write-Host "FAILED: $model" -ForegroundColor Red
        Write-Host "Error: $errorMsg" -ForegroundColor Red
    }
}

