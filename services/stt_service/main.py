"""
FastAPI service for speech-to-text using faster-whisper
Note: If faster-whisper installation fails on Python 3.14, you can use openai-whisper as fallback
"""
from fastapi import FastAPI, File, UploadFile, HTTPException
from fastapi.responses import JSONResponse
import tempfile
import os
import logging
import time
import torch

# Import faster-whisper (primary, faster)
# Fallback to openai-whisper if faster-whisper is not available
USE_FASTER_WHISPER = True
try:
    from faster_whisper import WhisperModel
    logging.info("Using faster-whisper (faster)")
except ImportError:
    try:
        import whisper
        USE_FASTER_WHISPER = False
        logging.warning("faster-whisper not available, using openai-whisper (slower)")
    except ImportError:
        raise ImportError("Neither faster-whisper nor openai-whisper is installed. Please install one of them.")

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

app = FastAPI(title="STT Service", version="1.0.0")

# Initialize Whisper model
# Use "tiny" model for maximum speed (3-5x faster than "base")
# Can be changed to "base", "small", "medium", "large" for better accuracy but slower
# Model is loaded on startup to avoid cold start
# Try to use GPU if available, fallback to CPU
device = "cuda" if torch.cuda.is_available() else "cpu"

logger.info(f"Loading Whisper model on {device}...")
if device == "cuda":
    logger.info(f"CUDA available - GPU: {torch.cuda.get_device_name(0)}")

if USE_FASTER_WHISPER:
    compute_type = "float16" if device == "cuda" else "int8"
    # Use "tiny" model for fastest transcription (3-5x faster than "base")
    model = WhisperModel("tiny", device=device, compute_type=compute_type)
else:
    # openai-whisper uses different device format
    model = whisper.load_model("tiny", device=device)
    
logger.info(f"Whisper model loaded and ready on {device}")

@app.post("/transcribe")
async def transcribe(audio: UploadFile = File(...)):
    """
    Transcribe audio file to text.
    Accepts WAV format, PCM 16-bit, 48kHz, mono.
    Uses streaming for faster response.
    """
    try:
        # Read audio file
        audio_data = await audio.read()
        
        # Save to temporary file (faster-whisper works better with files)
        with tempfile.NamedTemporaryFile(suffix=".wav", delete=False) as tmp_file:
            tmp_file.write(audio_data)
            tmp_path = tmp_file.name
        
        try:
            if USE_FASTER_WHISPER:
                # Use streaming transcription for faster response
                # Optimized for maximum speed
                segments, info = model.transcribe(
                    tmp_path,
                    language="en",
                    beam_size=1,  # Minimum beam size for speed
                    vad_filter=True,  # Use voice activity detection
                    condition_on_previous_text=False,  # Faster, no context dependency
                    initial_prompt=None,  # No prompt for speed
                    word_timestamps=False,  # Skip word timestamps for speed
                    temperature=0,  # Deterministic, faster
                    compression_ratio_threshold=2.4,  # Default, but explicit
                    log_prob_threshold=-1.0,  # Default, but explicit
                    no_speech_threshold=0.6,  # Default, but explicit
                )
                
                # Stream segments as they come (faster first response)
                # Collect segments efficiently
                text_parts = []
                first_segment_time = None
                segment_count = 0
                for segment in segments:
                    if first_segment_time is None:
                        first_segment_time = time.time()
                    text_parts.append(segment.text.strip())
                    segment_count += 1
                    # Log first segment immediately for debugging
                    if segment_count == 1:
                        logger.info(f"First transcription segment (t={time.time()-first_segment_time:.2f}s): {segment.text}")
                
                # Join efficiently
                full_text = " ".join(text_parts).strip()
                language = info.language
                language_probability = info.language_probability
                
                if first_segment_time:
                    transcription_time = time.time() - first_segment_time
                    logger.info(f"Transcription completed in {transcription_time:.2f}s ({segment_count} segments): {full_text}")
            else:
                # Transcribe using openai-whisper
                result = model.transcribe(tmp_path, language="en")
                full_text = result["text"].strip()
                language = result.get("language", "en")
                language_probability = 1.0  # openai-whisper doesn't provide this
            
            return JSONResponse(content={
                "text": full_text,
                "language": language,
                "language_probability": language_probability,
            })
        finally:
            # Clean up temp file
            try:
                os.unlink(tmp_path)
            except:
                pass
    
    except Exception as e:
        logger.error(f"Transcription error: {str(e)}", exc_info=True)
        raise HTTPException(status_code=500, detail=f"Transcription failed: {str(e)}")

@app.get("/health")
async def health():
    """Health check endpoint"""
    return {"status": "healthy", "service": "stt", "device": device}

@app.post("/warmup")
async def warmup():
    """
    Warmup endpoint to ensure model is fully loaded and ready.
    Performs a small transcription to initialize all model components.
    """
    try:
        # Create a minimal audio file (silence) for warmup
        import wave
        import numpy as np
        
        # Generate 0.5 seconds of silence (16-bit PCM, 48kHz, mono)
        sample_rate = 48000
        duration = 0.5
        samples = np.zeros(int(sample_rate * duration), dtype=np.int16)
        
        with tempfile.NamedTemporaryFile(suffix=".wav", delete=False) as tmp_file:
            tmp_path = tmp_file.name
            with wave.open(tmp_path, 'wb') as wav_file:
                wav_file.setnchannels(1)  # Mono
                wav_file.setsampwidth(2)  # 16-bit
                wav_file.setframerate(sample_rate)
                wav_file.writeframes(samples.tobytes())
        
        try:
            # Perform a quick transcription to warm up the model
            if USE_FASTER_WHISPER:
                segments, info = model.transcribe(
                    tmp_path,
                    language="en",
                    beam_size=1,
                    vad_filter=True,
                )
                # Consume at least one segment to ensure model is active
                _ = next(segments, None)
            else:
                # For openai-whisper, just transcribe
                _ = model.transcribe(tmp_path, language="en")
            
            logger.info("STT model warmed up successfully")
            return {"status": "warmed_up", "service": "stt", "device": device}
        finally:
            try:
                os.unlink(tmp_path)
            except:
                pass
    except Exception as e:
        logger.error(f"STT warmup failed: {e}", exc_info=True)
        return {"status": "warmup_failed", "error": str(e)}

if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8001)
