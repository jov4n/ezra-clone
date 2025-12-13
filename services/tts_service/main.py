"""
FastAPI service for text-to-speech using XTTS V2 with streaming support
"""
from fastapi import FastAPI, HTTPException
from fastapi.responses import Response, FileResponse, StreamingResponse
from pydantic import BaseModel
import os
import tempfile
import logging
import subprocess
import sys
import torch
import wave
import numpy as np
import asyncio

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

# Configuration
REFERENCE_AUDIO_DIR = os.getenv("REFERENCE_AUDIO_DIR", "/app/references")

# Detect GPU availability
device = "cuda" if torch.cuda.is_available() else "cpu"
if device == "cuda":
    logger.info(f"CUDA available for TTS - GPU: {torch.cuda.get_device_name(0)}")
else:
    logger.info("CUDA not available for TTS, using CPU.")
logger.info(f"TTS service initialized - using device: {device}")

# XTTS V2 will be loaded on first request (lazy loading)
# Set environment variable to auto-accept TOS for non-interactive use
os.environ["COQUI_TOS_AGREED"] = "1"

xtts_available = False
try:
    from TTS.api import TTS
    xtts_available = True
    logger.info(f"XTTS V2 available (will load on first request, using {device})")
except ImportError:
    logger.warning("XTTS V2 not available. Install with: pip install TTS")

def generate_simple_wav(text: str, sample_rate: int = 22050, duration: float = None) -> bytes:
    """
    Generate a simple WAV file with a tone (fallback when XTTS V2 is not available).
    This creates a basic audio file that can be used for testing.
    """
    if duration is None:
        # Estimate duration: ~150 words per minute, ~5 characters per word
        duration = max(1.0, len(text) / (150 * 5 / 60))
    
    # Generate a simple tone (440 Hz sine wave) as placeholder
    # In production, this would be replaced with actual TTS
    t = np.linspace(0, duration, int(sample_rate * duration), False)
    frequency = 440.0
    audio_data = np.sin(2 * np.pi * frequency * t)
    
    # Normalize to 16-bit PCM
    audio_data = (audio_data * 32767).astype(np.int16)
    
    # Create WAV file in memory using BytesIO to avoid file locking issues
    import io
    wav_buffer = io.BytesIO()
    
    with wave.open(wav_buffer, 'wb') as wav_file:
        wav_file.setnchannels(1)  # Mono
        wav_file.setsampwidth(2)  # 16-bit
        wav_file.setframerate(sample_rate)
        wav_file.writeframes(audio_data.tobytes())
    
    # Get the WAV data from the buffer
    wav_data = wav_buffer.getvalue()
    wav_buffer.close()
    
    return wav_data

from contextlib import asynccontextmanager

@asynccontextmanager
async def lifespan(app: FastAPI):
    """Lifespan event handler for startup/shutdown"""
    # Startup
    logger.info("Warming up TTS service with XTTS V2...")
    try:
        # Try to load XTTS V2 model during warmup
        warmup_text = "Hi"
        
        # Try to find default reference audio
        default_ref = ""
        current_dir = os.path.dirname(os.path.abspath(__file__))
        project_root = os.path.dirname(os.path.dirname(current_dir))
        possible_paths = [
            os.path.join(REFERENCE_AUDIO_DIR, "default.wav"),
            os.path.join(project_root, "voice_references", "default.wav"),
            "../../voice_references/default.wav",
            "../voice_references/default.wav",
            "voice_references/default.wav",
        ]
        
        for path in possible_paths:
            abs_path = os.path.abspath(path) if not os.path.isabs(path) else path
            if os.path.exists(abs_path):
                default_ref = abs_path
                logger.info(f"Found reference audio: {abs_path}")
                break
        
        # Actually load XTTS V2 model during warmup
        if xtts_available and default_ref:
            try:
                from xtts_integration import synthesize_with_xtts, XTTS_AVAILABLE
                if XTTS_AVAILABLE:
                    logger.info("Loading XTTS V2 model during warmup...")
                    synthesize_with_xtts(
                        text=warmup_text,
                        reference_audio_path=default_ref,
                        device=device,
                        language="en",
                        stream=False,
                    )
                    logger.info("✓ XTTS V2 model loaded and warmed up successfully")
                else:
                    logger.warning("XTTS V2 not available for warmup")
            except Exception as warmup_error:
                logger.warning(f"XTTS V2 warmup failed: {warmup_error} (will load on first request)")
        else:
            logger.info("XTTS V2 warmup skipped (will load on first request)")
    except Exception as e:
        logger.warning(f"TTS warmup failed (this is okay): {str(e)}")
        logger.info("TTS service will load on first request")
    
    yield
    
    # Shutdown (if needed)
    logger.info("TTS service shutting down")

# Create app with lifespan handler
app = FastAPI(title="TTS Service", version="1.0.0", lifespan=lifespan)

class SynthesizeRequest(BaseModel):
    text: str
    reference_path: str
    stream: bool = False  # Whether to stream chunks

@app.post("/synthesize")
async def synthesize(request: SynthesizeRequest):
    """
    Synthesize speech from text using XTTS V2 with zero-shot voice cloning.
    
    Args:
        text: Text to synthesize
        reference_path: Path to reference audio file for voice cloning (6+ seconds recommended)
    
    Returns:
        Audio file (WAV format)
    """
    try:
        logger.info(f"Synthesizing text: '{request.text[:50]}...' with reference: {request.reference_path}")
        
        # Resolve reference path - it might be relative to project root, need to convert to TTS service relative
        resolved_ref_path = request.reference_path
        if request.reference_path:
            # Get the project root (two levels up from services/tts_service)
            current_dir = os.path.dirname(os.path.abspath(__file__))
            project_root = os.path.dirname(os.path.dirname(current_dir))
            
            # Try to resolve the path
            if not os.path.isabs(request.reference_path):
                # It's a relative path - try from project root
                project_root_path = os.path.join(project_root, request.reference_path)
                if os.path.exists(project_root_path):
                    resolved_ref_path = os.path.abspath(project_root_path)
                    logger.info(f"Resolved reference path from project root: {resolved_ref_path}")
                else:
                    # Try relative to current directory
                    current_dir_path = os.path.join(current_dir, request.reference_path)
                    if os.path.exists(current_dir_path):
                        resolved_ref_path = os.path.abspath(current_dir_path)
                        logger.info(f"Resolved reference path from current dir: {resolved_ref_path}")
                    else:
                        # Try other possible locations
                        possible_paths = [
                            os.path.join(project_root, request.reference_path),
                            os.path.join(current_dir, "..", "..", request.reference_path),
                            os.path.join(current_dir, "..", request.reference_path),
                            request.reference_path,
                        ]
                        for path in possible_paths:
                            abs_path = os.path.abspath(path)
                            if os.path.exists(abs_path):
                                resolved_ref_path = abs_path
                                logger.info(f"Found reference at: {resolved_ref_path}")
                                break
                        else:
                            logger.warning(f"Reference audio file not found: {request.reference_path}, using default")
                            resolved_ref_path = ""
            else:
                # Absolute path - use as is
                if os.path.exists(request.reference_path):
                    resolved_ref_path = request.reference_path
                    logger.info(f"Using absolute reference path: {resolved_ref_path}")
                else:
                    logger.warning(f"Reference audio file not found: {request.reference_path}, using default")
                    resolved_ref_path = ""
        
        # Create temporary output file
        with tempfile.NamedTemporaryFile(suffix=".wav", delete=False) as tmp_file:
            output_path = tmp_file.name
        
        try:
            # Try to use XTTS V2 if available
            try:
                from xtts_integration import synthesize_with_xtts, XTTS_AVAILABLE
                
                if XTTS_AVAILABLE:
                    logger.info("=" * 60)
                    logger.info("Using XTTS V2 for TTS synthesis with voice cloning")
                    logger.info(f"Text: '{request.text[:100]}...'")
                    logger.info(f"Reference: {resolved_ref_path if resolved_ref_path else 'None (using default voice)'}")
                    logger.info("=" * 60)
                    # Use streaming if requested
                    if request.stream:
                        # Return streaming response
                        def generate_audio_chunks():
                            try:
                                from xtts_integration import synthesize_with_xtts_streaming
                                for chunk in synthesize_with_xtts_streaming(
                                    text=request.text,
                                    reference_audio_path=resolved_ref_path if resolved_ref_path else None,
                                    device=device,
                                    language="en",
                                ):
                                    yield chunk
                            except Exception as e:
                                logger.error(f"Streaming error: {e}", exc_info=True)
                                # Fallback: return full audio
                                audio_data = synthesize_with_xtts(
                                    text=request.text,
                                    reference_audio_path=resolved_ref_path if resolved_ref_path else None,
                                    output_path=None,
                                    device=device,
                                    language="en",
                                    stream=False,
                                )
                                yield audio_data
                        
                        return StreamingResponse(
                            generate_audio_chunks(),
                            media_type="audio/wav",
                            headers={
                                "Content-Disposition": f'attachment; filename="synthesized.wav"',
                                "X-Content-Type-Options": "nosniff",
                            }
                        )
                    else:
                        # Non-streaming: generate full audio
                        audio_data = synthesize_with_xtts(
                            text=request.text,
                            reference_audio_path=resolved_ref_path if resolved_ref_path else None,
                            output_path=output_path,
                            device=device,
                            language="en",
                            stream=False,
                        )
                        
                        logger.info(f"✓ XTTS V2 synthesis complete: {len(audio_data)} bytes")
                        return Response(
                            content=audio_data,
                            media_type="audio/wav",
                            headers={
                                "Content-Disposition": f'attachment; filename="synthesized.wav"'
                            }
                        )
                else:
                    logger.info("XTTS V2 not available, using simple TTS fallback")
            except ImportError as e:
                # XTTS V2 not available - use fallback
                logger.info(f"XTTS V2 not available ({e}), using simple TTS fallback")
            except Exception as e:
                logger.warning(f"XTTS V2 synthesis failed: {e}, using fallback")
                # Fall through to fallback
            
            # Fallback: Generate simple WAV file
            # This ensures the endpoint always works, even without XTTS V2
            audio_data = generate_simple_wav(request.text)
            logger.info(f"Generated fallback audio ({len(audio_data)} bytes)")
            
            return Response(
                content=audio_data,
                media_type="audio/wav",
                headers={
                    "Content-Disposition": f'attachment; filename="synthesized.wav"'
                }
            )
        
        finally:
            # Clean up temp file
            if os.path.exists(output_path):
                try:
                    os.unlink(output_path)
                except:
                    pass
    
    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"TTS synthesis error: {str(e)}", exc_info=True)
        raise HTTPException(status_code=500, detail=f"TTS synthesis failed: {str(e)}")

@app.get("/health")
async def health():
    """Health check endpoint"""
    return {"status": "healthy", "service": "tts", "device": device}

@app.post("/warmup")
async def warmup():
    """
    Warmup endpoint to ensure XTTS V2 model is fully loaded and ready.
    Performs a small synthesis to initialize all model components.
    """
    try:
        # Try to find default reference audio
        default_ref = ""
        current_dir = os.path.dirname(os.path.abspath(__file__))
        project_root = os.path.dirname(os.path.dirname(current_dir))
        possible_paths = [
            os.path.join(REFERENCE_AUDIO_DIR, "default.wav"),
            os.path.join(project_root, "voice_references", "default.wav"),
            "../../voice_references/default.wav",
            "../voice_references/default.wav",
            "voice_references/default.wav",
        ]
        
        for path in possible_paths:
            abs_path = os.path.abspath(path) if not os.path.isabs(path) else path
            if os.path.exists(abs_path):
                default_ref = abs_path
                break
        
        if not default_ref:
            return {"status": "warmup_skipped", "reason": "no_reference_audio"}
        
        # Perform a small synthesis to warm up XTTS V2
        if xtts_available:
            try:
                from xtts_integration import synthesize_with_xtts, XTTS_AVAILABLE
                if XTTS_AVAILABLE:
                    logger.info("Warming up XTTS V2 model with small synthesis...")
                    warmup_text = "Hi"
                    _ = synthesize_with_xtts(
                        text=warmup_text,
                        reference_audio_path=default_ref,
                        device=device,
                        language="en",
                        stream=False,
                    )
                    logger.info("✓ XTTS V2 model warmed up successfully")
                    return {"status": "warmed_up", "service": "tts", "device": device}
                else:
                    return {"status": "warmup_skipped", "reason": "xtts_not_available"}
            except Exception as e:
                logger.error(f"XTTS V2 warmup failed: {e}", exc_info=True)
                return {"status": "warmup_failed", "error": str(e)}
        else:
            return {"status": "warmup_skipped", "reason": "xtts_not_available"}
    except Exception as e:
        logger.error(f"TTS warmup error: {e}", exc_info=True)
        return {"status": "warmup_failed", "error": str(e)}

if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8002)

