FROM python:3.11-slim

WORKDIR /app

# Install system dependencies
RUN apt-get update && apt-get install -y \
    ffmpeg \
    git \
    && rm -rf /var/lib/apt/lists/*

# Install VibeVoice
# Note: This is a placeholder - adjust based on actual VibeVoice installation
# RUN git clone https://github.com/microsoft/VibeVoice.git /app/vibevoice
# RUN cd /app/vibevoice && pip install -e .

# Copy requirements
COPY requirements.txt .

# Install Python dependencies
RUN pip install --no-cache-dir -r requirements.txt

# Copy application
COPY main.py .

# Create references directory
RUN mkdir -p /app/references

# Expose port
EXPOSE 8002

# Run the application
CMD ["python", "main.py"]

