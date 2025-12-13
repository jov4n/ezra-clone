# Setup script for voice services with UV and Python 3.12
# This script installs Python 3.12, creates venvs, and installs all dependencies with GPU support

$env:Path = "C:\Users\jovan\.local\bin;$env:Path"

Write-Host "Setting up voice services with UV..." -ForegroundColor Green

# Install Python 3.12 if not already installed
Write-Host "Installing Python 3.12..." -ForegroundColor Yellow
uv python install 3.12

# Create virtual environments
Write-Host "Creating virtual environments..." -ForegroundColor Yellow
uv venv services/stt_service/.venv --python 3.12
uv venv services/tts_service/.venv --python 3.12

# Install PyTorch with CUDA support
Write-Host "Installing PyTorch with CUDA 12.1 support..." -ForegroundColor Yellow
uv pip install --python services/stt_service/.venv/Scripts/python.exe torch torchvision torchaudio --index-url https://download.pytorch.org/whl/cu121
uv pip install --python services/tts_service/.venv/Scripts/python.exe torch torchvision torchaudio --index-url https://download.pytorch.org/whl/cu121

# Install STT service dependencies
Write-Host "Installing STT service dependencies..." -ForegroundColor Yellow
uv pip install --python services/stt_service/.venv/Scripts/python.exe faster-whisper
uv pip install --python services/stt_service/.venv/Scripts/python.exe -r services/stt_service/requirements.txt

# Install TTS service dependencies
Write-Host "Installing TTS service dependencies..." -ForegroundColor Yellow
uv pip install --python services/tts_service/.venv/Scripts/python.exe -r services/tts_service/requirements.txt

Write-Host "Setup complete! Services are ready to use with GPU support." -ForegroundColor Green

