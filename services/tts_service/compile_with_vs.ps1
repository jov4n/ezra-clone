# Script to compile APEX and Flash Attention with Visual Studio compiler

Write-Host "=== Setting up compilation environment ===" -ForegroundColor Cyan

# Set CUDA environment
$env:CUDA_HOME = "C:\Program Files\NVIDIA GPU Computing Toolkit\CUDA\v13.1"
$env:PATH = "$env:CUDA_HOME\bin;$env:CUDA_HOME\libnvvp;$env:PATH"

# Find Visual Studio compiler
$vsPaths = @(
    "C:\Program Files\Microsoft Visual Studio\2022\Community\VC\Auxiliary\Build\vcvarsall.bat",
    "C:\Program Files\Microsoft Visual Studio\2022\Professional\VC\Auxiliary\Build\vcvarsall.bat",
    "C:\Program Files\Microsoft Visual Studio\2022\Enterprise\VC\Auxiliary\Build\vcvarsall.bat",
    "C:\Program Files\Microsoft Visual Studio\2022\BuildTools\VC\Auxiliary\Build\vcvarsall.bat",
    "C:\Program Files (x86)\Microsoft Visual Studio\2022\BuildTools\VC\Auxiliary\Build\vcvarsall.bat"
)

$vcvars = $null
foreach ($path in $vsPaths) {
    if (Test-Path $path) {
        $vcvars = $path
        break
    }
}

if (-not $vcvars) {
    Write-Host "ERROR: Visual Studio C++ compiler not found!" -ForegroundColor Red
    Write-Host "Please install Visual Studio Build Tools 2022 with 'Desktop development with C++' workload" -ForegroundColor Yellow
    Write-Host "Download: https://visualstudio.microsoft.com/downloads/" -ForegroundColor Yellow
    exit 1
}

Write-Host "Found Visual Studio at: $vcvars" -ForegroundColor Green

# Compile APEX
Write-Host "`n=== Compiling APEX (10-20 minutes) ===" -ForegroundColor Cyan
$apexCmd = "`"$vcvars`" x64 && cd C:\apex && `"C:\Users\jovan\OneDrive\Desktop\Ezra Clone\services\tts_service\.venv\Scripts\python.exe`" setup.py install --cpp_ext --cuda_ext"
cmd /c $apexCmd

if ($LASTEXITCODE -eq 0) {
    Write-Host "`n✓ APEX compiled successfully!" -ForegroundColor Green
} else {
    Write-Host "`n✗ APEX compilation failed" -ForegroundColor Red
    exit 1
}

# Compile Flash Attention
Write-Host "`n=== Compiling Flash Attention (5-15 minutes) ===" -ForegroundColor Cyan
cd "C:\Users\jovan\OneDrive\Desktop\Ezra Clone\services\tts_service"
$flashCmd = "`"$vcvars`" x64 && uv pip install --python .venv\Scripts\python.exe flash-attn --no-build-isolation"
cmd /c $flashCmd

if ($LASTEXITCODE -eq 0) {
    Write-Host "`n✓ Flash Attention compiled successfully!" -ForegroundColor Green
} else {
    Write-Host "`n✗ Flash Attention compilation failed" -ForegroundColor Red
    exit 1
}

Write-Host "`n=== All optimizations installed! ===" -ForegroundColor Green



