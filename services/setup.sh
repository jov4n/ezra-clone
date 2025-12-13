#!/bin/bash
# Setup script for voice services with UV and Python 3.12
# This script installs Python 3.12, creates venvs, and installs all dependencies with GPU support

set -e

UV_PATH="$HOME/.local/bin"
export PATH="$UV_PATH:$PATH"

echo "Setting up voice services with UV..."

# Install Python 3.12 if not already installed
echo "Installing Python 3.12..."
uv python install 3.12

# Create virtual environments
echo "Creating virtual environments..."
uv venv services/stt_service/.venv --python 3.12
uv venv services/tts_service/.venv --python 3.12

# Install PyTorch with CUDA support
echo "Installing PyTorch with CUDA 12.1 support..."
uv pip install --python services/stt_service/.venv/Scripts/python.exe torch torchvision torchaudio --index-url https://download.pytorch.org/whl/cu121
uv pip install --python services/tts_service/.venv/Scripts/python.exe torch torchvision torchaudio --index-url https://download.pytorch.org/whl/cu121

# Install STT service dependencies
echo "Installing STT service dependencies..."
uv pip install --python services/stt_service/.venv/Scripts/python.exe faster-whisper
uv pip install --python services/stt_service/.venv/Scripts/python.exe -r services/stt_service/requirements.txt

# Install TTS service dependencies
echo "Installing TTS service dependencies..."
uv pip install --python services/tts_service/.venv/Scripts/python.exe -r services/tts_service/requirements.txt

echo "Setup complete! Services are ready to use with GPU support."

