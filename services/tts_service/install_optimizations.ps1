# Script to install APEX and Flash Attention optimizations
# Requires: CUDA Toolkit 12.1 and Visual Studio Build Tools

Write-Host "=== Installing APEX and Flash Attention ===" -ForegroundColor Cyan

# Check for CUDA Toolkit
$cudaPaths = @(
    "C:\Program Files\NVIDIA GPU Computing Toolkit\CUDA\v12.1",
    "C:\Program Files\NVIDIA GPU Computing Toolkit\CUDA\v12.2",
    "C:\Program Files\NVIDIA GPU Computing Toolkit\CUDA\v12.0"
)

$cudaHome = $null
foreach ($path in $cudaPaths) {
    if (Test-Path $path) {
        $cudaHome = $path
        break
    }
}

if (-not $cudaHome) {
    Write-Host "ERROR: CUDA Toolkit not found!" -ForegroundColor Red
    Write-Host "Please install CUDA Toolkit 12.1 from:" -ForegroundColor Yellow
    Write-Host "https://developer.nvidia.com/cuda-12-1-0-download-archive" -ForegroundColor Yellow
    exit 1
}

Write-Host "Found CUDA at: $cudaHome" -ForegroundColor Green

# Set environment variables
$env:CUDA_HOME = $cudaHome
$env:PATH = "$cudaHome\bin;$cudaHome\libnvvp;$env:PATH"

# Verify nvcc
$nvccPath = "$cudaHome\bin\nvcc.exe"
if (-not (Test-Path $nvccPath)) {
    Write-Host "ERROR: nvcc not found at $nvccPath" -ForegroundColor Red
    Write-Host "CUDA Toolkit may not be properly installed" -ForegroundColor Yellow
    exit 1
}

Write-Host "nvcc found! Verifying..." -ForegroundColor Green
& $nvccPath --version

# Install APEX
Write-Host "`n=== Installing APEX ===" -ForegroundColor Cyan
if (-not (Test-Path "C:\apex")) {
    Write-Host "Cloning APEX repository..." -ForegroundColor Yellow
    git clone https://github.com/NVIDIA/apex.git C:\apex
}

cd C:\apex
Write-Host "Compiling APEX (this may take 10-20 minutes)..." -ForegroundColor Yellow
& "C:\Users\jovan\OneDrive\Desktop\Ezra Clone\services\tts_service\.venv\Scripts\python.exe" setup.py install --cpp_ext --cuda_ext

# Install Flash Attention
Write-Host "`n=== Installing Flash Attention ===" -ForegroundColor Cyan
cd "C:\Users\jovan\OneDrive\Desktop\Ezra Clone\services\tts_service"
uv pip install --python .venv\Scripts\python.exe flash-attn --no-build-isolation

Write-Host "`n=== Installation Complete ===" -ForegroundColor Green
Write-Host "Restart your bot to use the optimizations!" -ForegroundColor Cyan



