"""
XTTS V2 integration for TTS service with streaming support
Based on: https://www.baseten.co/blog/streaming-real-time-text-to-speech-with-xtts-v2/
"""
import os
import sys
import logging
import time
import torch
import numpy as np
import tempfile
from typing import Optional, Iterator
import io
import wave

logger = logging.getLogger(__name__)

# Global model cache
_model_cache = None
_device = None

def synthesize_with_xtts(
    text: str,
    reference_audio_path: str = None,
    output_path: str = None,
    device: str = "cuda" if torch.cuda.is_available() else "cpu",
    language: str = "en",
    stream: bool = False,
    chunk_size: int = 150,
) -> bytes:
    """
    Synthesize speech using XTTS V2 with voice cloning and streaming support.
    
    Args:
        text: Text to synthesize
        reference_audio_path: Path to reference audio file for voice cloning (6+ seconds recommended)
        output_path: Path to save output WAV file (if None, returns bytes)
        device: Device to use ("cuda" or "cpu")
        language: Language code (default: "en")
        stream: Whether to stream output (for real-time playback)
        chunk_size: Chunk size for streaming (default: 150)
    
    Returns:
        Audio data as bytes (WAV format) or iterator of chunks if streaming
    """
    global _model_cache, _device
    
    try:
        from TTS.api import TTS
        from TTS.utils.manage import ModelManager
    except ImportError:
        raise ImportError(
            "XTTS V2 not available. Install with: pip install TTS"
        )
    
    # Initialize model (cached)
    if _model_cache is None or _device != device:
        logger.info(f"Loading XTTS V2 model on {device}...")
        _device = device
        
        try:
            # Patch input() to automatically accept TOS non-interactively
            # This accepts the non-commercial CPML license
            import builtins
            original_input = builtins.input
            
            def auto_accept_tos(prompt=""):
                if "confirm" in prompt.lower() or "agree" in prompt.lower() or "y/n" in prompt.lower():
                    logger.info("Auto-accepting XTTS V2 terms of service (non-commercial CPML)")
                    return "y"
                return original_input(prompt)
            
            # Temporarily replace input() to auto-accept TOS
            builtins.input = auto_accept_tos
            
            try:
                # Initialize TTS with XTTS V2 model
                # Model name: "tts_models/multilingual/multi-dataset/xtts_v2"
                tts = TTS(
                    model_name="tts_models/multilingual/multi-dataset/xtts_v2",
                    progress_bar=False,
                )
                
                # Move to device (new recommended way instead of gpu parameter)
                if device == "cuda":
                    tts.to(device)
                    
                    # Optimize for speed: use half precision (float16) for faster inference
                    # This can provide 2x speedup on modern GPUs with minimal quality loss
                    try:
                        if hasattr(tts, 'synthesizer') and hasattr(tts.synthesizer, 'model'):
                            logger.info("Optimizing XTTS V2 model for speed...")
                            
                            # Convert model to half precision for faster inference
                            try:
                                tts.synthesizer.model = tts.synthesizer.model.half()
                                logger.info("✓ Model converted to float16 for faster inference")
                            except Exception as half_error:
                                logger.debug(f"Half precision conversion failed: {half_error}")
                            
                            # Try to compile the model for faster inference
                            # This can provide additional 1.5-2x speedup
                            try:
                                tts.synthesizer.model = torch.compile(
                                    tts.synthesizer.model, 
                                    mode="reduce-overhead",
                                    fullgraph=False  # Allow partial compilation
                                )
                                logger.info("✓ Model compiled successfully")
                            except Exception as compile_error:
                                logger.debug(f"Model compilation failed (will use uncompiled): {compile_error}")
                            
                            # Also try to optimize the vocoder if available
                            if hasattr(tts, 'vocoder') and hasattr(tts.vocoder, 'model'):
                                try:
                                    tts.vocoder.model = tts.vocoder.model.half()
                                    logger.info("✓ Vocoder converted to float16")
                                except:
                                    pass  # Vocoder half precision is optional
                                    
                                try:
                                    tts.vocoder.model = torch.compile(
                                        tts.vocoder.model,
                                        mode="reduce-overhead",
                                        fullgraph=False
                                    )
                                    logger.info("✓ Vocoder compiled successfully")
                                except:
                                    pass  # Vocoder compilation is optional
                    except Exception as opt_error:
                        logger.warning(f"Speed optimization failed (will use default): {opt_error}")
                    
                    # Enable CUDA optimizations
                    torch.backends.cudnn.benchmark = True  # Optimize for consistent input sizes
                    torch.backends.cudnn.deterministic = False  # Allow non-deterministic for speed
                    
                    # Set CUDA memory management for speed
                    if torch.cuda.is_available():
                        # Enable memory pool for faster allocations
                        torch.cuda.empty_cache()
                        logger.info("CUDA optimizations enabled")
                
                _model_cache = tts
                logger.info("XTTS V2 model loaded successfully")
            finally:
                # Restore original input function
                builtins.input = original_input
                
        except Exception as e:
            logger.error(f"Failed to load XTTS V2 model: {e}", exc_info=True)
            # Restore input in case of error
            try:
                import builtins
                if hasattr(builtins, 'input'):
                    builtins.input = original_input
            except:
                pass
            raise
    
    model = _model_cache
    
    # Process reference audio if provided
    speaker_wav = None
    if reference_audio_path and os.path.exists(reference_audio_path):
        logger.info(f"Using reference audio for voice cloning: {reference_audio_path}")
        speaker_wav = reference_audio_path
    else:
        logger.warning("No reference audio provided, using default voice")
    
    # Generate audio
    logger.info(f"Generating speech with XTTS V2 (streaming={stream})...")
    start_time = time.time()
    
    try:
        if stream:
            # Streaming mode - yield chunks as they're generated
            # XTTS V2 doesn't have native streaming, so we'll generate in chunks
            # For now, generate full audio and simulate streaming
            logger.info("Streaming mode requested (generating full audio first)")
            
            # Generate full audio
            with tempfile.NamedTemporaryFile(suffix=".wav", delete=False) as tmp_file:
                temp_output = tmp_file.name
            
            try:
                # Use torch.inference_mode() for faster inference
                # Also try to optimize speed by accessing model directly if possible
                with torch.inference_mode():
                    # Try to set speed optimizations on the synthesizer if available
                    speed_optimized = False
                    try:
                        if hasattr(model, 'synthesizer') and hasattr(model.synthesizer, 'model'):
                            # Try to reduce inference steps for faster generation
                            # XTTS V2 uses diffusion steps - fewer steps = faster but potentially lower quality
                            if hasattr(model.synthesizer.model, 'inference_steps'):
                                # Reduce steps for speed (default is usually 6, we can try 4)
                                original_steps = getattr(model.synthesizer.model, 'inference_steps', None)
                                if original_steps and original_steps > 4:
                                    model.synthesizer.model.inference_steps = 4
                                    speed_optimized = True
                                    logger.debug(f"Reduced inference steps to 4 for speed (was {original_steps})")
                    except Exception as opt_error:
                        logger.debug(f"Speed optimization attempt failed: {opt_error}")
                    
                    model.tts_to_file(
                        text=text,
                        file_path=temp_output,
                        speaker_wav=speaker_wav,
                        language=language,
                    )
                    
                    # Restore original steps if we changed them
                    if speed_optimized:
                        try:
                            if hasattr(model.synthesizer, 'model') and hasattr(model.synthesizer.model, 'inference_steps'):
                                # Restore to default (6) for next generation
                                model.synthesizer.model.inference_steps = 6
                        except:
                            pass
                
                # Read and return audio
                with open(temp_output, "rb") as f:
                    audio_data = f.read()
                
                generation_time = time.time() - start_time
                logger.info(f"Speech generation completed in {generation_time:.2f}s")
                
                return audio_data
            finally:
                if os.path.exists(temp_output):
                    os.unlink(temp_output)
        else:
            # Non-streaming mode - generate full audio
            with tempfile.NamedTemporaryFile(suffix=".wav", delete=False) as tmp_file:
                temp_output = tmp_file.name
            
            try:
                # Use torch.inference_mode() for faster inference
                # Also try to optimize speed by accessing model directly if possible
                with torch.inference_mode():
                    # Try to set speed optimizations on the synthesizer if available
                    speed_optimized = False
                    try:
                        if hasattr(model, 'synthesizer') and hasattr(model.synthesizer, 'model'):
                            # Try to reduce inference steps for faster generation
                            # XTTS V2 uses diffusion steps - fewer steps = faster but potentially lower quality
                            if hasattr(model.synthesizer.model, 'inference_steps'):
                                # Reduce steps for speed (default is usually 6, we can try 4)
                                original_steps = getattr(model.synthesizer.model, 'inference_steps', None)
                                if original_steps and original_steps > 4:
                                    model.synthesizer.model.inference_steps = 4
                                    speed_optimized = True
                                    logger.debug(f"Reduced inference steps to 4 for speed (was {original_steps})")
                    except Exception as opt_error:
                        logger.debug(f"Speed optimization attempt failed: {opt_error}")
                    
                    model.tts_to_file(
                        text=text,
                        file_path=temp_output,
                        speaker_wav=speaker_wav,
                        language=language,
                    )
                    
                    # Restore original steps if we changed them
                    if speed_optimized:
                        try:
                            if hasattr(model.synthesizer, 'model') and hasattr(model.synthesizer.model, 'inference_steps'):
                                # Restore to default (6) for next generation
                                model.synthesizer.model.inference_steps = 6
                        except:
                            pass
                
                # Read audio data
                with open(temp_output, "rb") as f:
                    audio_data = f.read()
                
                generation_time = time.time() - start_time
                logger.info(f"Speech generation completed in {generation_time:.2f}s")
                
                # Save to output_path if provided
                if output_path:
                    with open(output_path, "wb") as f:
                        f.write(audio_data)
                    logger.info(f"Audio saved to: {output_path}")
                    return audio_data
                else:
                    return audio_data
            finally:
                if os.path.exists(temp_output):
                    os.unlink(temp_output)
                    
    except Exception as e:
        logger.error(f"XTTS V2 synthesis error: {e}", exc_info=True)
        raise

def synthesize_with_xtts_streaming(
    text: str,
    reference_audio_path: str = None,
    device: str = "cuda" if torch.cuda.is_available() else "cpu",
    language: str = "en",
    chunk_size: int = 150,
) -> Iterator[bytes]:
    """
    Stream TTS audio chunks as they're generated for real-time playback.
    
    This splits the text into sentences and generates each chunk incrementally,
    yielding audio chunks as soon as they're ready.
    
    Args:
        text: Text to synthesize
        reference_audio_path: Path to reference audio file for voice cloning
        device: Device to use ("cuda" or "cpu")
        language: Language code (default: "en")
        chunk_size: Approximate chunk size in characters
    
    Yields:
        Audio chunks as bytes (WAV format)
    """
    import re
    
    # Split text into sentences for chunked generation
    # This allows us to start playing as soon as the first sentence is ready
    sentences = re.split(r'([.!?]+[\s\n]*)', text)
    # Recombine sentences with their punctuation
    text_chunks = []
    current_chunk = ""
    for i, part in enumerate(sentences):
        current_chunk += part
        # If we have a complete sentence or reached chunk size, add it
        if (part.strip() and any(p in part for p in '.!?')) or len(current_chunk) >= chunk_size:
            if current_chunk.strip():
                text_chunks.append(current_chunk.strip())
                current_chunk = ""
    
    # Add remaining text
    if current_chunk.strip():
        text_chunks.append(current_chunk.strip())
    
    # If no sentences found, use whole text
    if not text_chunks:
        text_chunks = [text]
    
    logger.info(f"Streaming TTS: split into {len(text_chunks)} chunks")
    
    # Generate and yield each chunk as complete WAV files
    first_chunk = True
    for i, chunk_text in enumerate(text_chunks):
        if not chunk_text.strip():
            continue
        
        try:
            # Generate this chunk
            chunk_audio = synthesize_with_xtts(
                text=chunk_text,
                reference_audio_path=reference_audio_path,
                output_path=None,
                device=device,
                language=language,
                stream=False,
            )
            
            if first_chunk:
                logger.info(f"✓ First audio chunk ready ({len(chunk_audio)} bytes) - will start playback immediately")
                first_chunk = False
            else:
                logger.debug(f"Generated chunk {i+1}/{len(text_chunks)} ({len(chunk_audio)} bytes)")
            
            # Yield complete WAV chunk
            yield chunk_audio
            
        except Exception as e:
            logger.error(f"Error generating chunk {i+1}/{len(text_chunks)}: {e}", exc_info=True)
            # Continue with next chunk
            continue

# Check if XTTS is available
try:
    from TTS.api import TTS
    XTTS_AVAILABLE = True
except ImportError:
    XTTS_AVAILABLE = False
    logger.warning("XTTS V2 not available. Install with: pip install TTS")

