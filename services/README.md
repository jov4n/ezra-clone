# Voice Services

This directory contains the STT (Speech-to-Text) and TTS (Text-to-Speech) microservices for the Discord bot.

## Setup

Both services use UV for package management and have their own virtual environments.

### Prerequisites

- Python 3.12 (installed automatically by UV if not present)
- UV package manager (installed at `C:\Users\jovan\.local\bin`)
- CUDA 12.1+ (for GPU acceleration - RTX 3080 detected)

### Installation

The virtual environments and dependencies are set up using UV with Python 3.12:

- **STT Service**: `services/stt_service/.venv` (uses faster-whisper with GPU support)
- **TTS Service**: `services/tts_service/.venv` (uses VibeVoice with GPU support)

## Running the Services

### STT Service (Port 8001)

**Windows (PowerShell):**
```powershell
cd services/stt_service
.\start.ps1
```

**Windows (CMD):**
```cmd
cd services/stt_service
start.bat
```

**Manual:**
```powershell
$env:Path = "C:\Users\jovan\.local\bin;$env:Path"
.venv\Scripts\Activate.ps1
python main.py
```

### TTS Service (Port 8002)

**Windows (PowerShell):**
```powershell
cd services/tts_service
.\start.ps1
```

**Windows (CMD):**
```cmd
cd services/tts_service
start.bat
```

**Manual:**
```powershell
$env:Path = "C:\Users\jovan\.local\bin;$env:Path"
.venv\Scripts\Activate.ps1
python main.py
```

## GPU Support

Both services are configured with PyTorch CUDA 12.1 support and will automatically use your RTX 3080:
- **STT**: Uses `float16` on GPU (faster-whisper with CUDA)
- **TTS**: Passes `--device cuda` to VibeVoice

The services use Python 3.12 with PyTorch CUDA wheels for full GPU acceleration.

Check the service logs on startup to see which device is being used (should show CUDA/GPU).

## Notes

- **Python 3.12**: Used for compatibility with PyTorch CUDA and faster-whisper
- **faster-whisper**: Installed and ready with GPU support
- **PyTorch CUDA**: Installed with CUDA 12.1 support
- Services will warm up models on startup to avoid cold start delays

