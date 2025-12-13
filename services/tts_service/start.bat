@echo off
REM Start TTS service using UV venv
set PATH=C:\Users\jovan\.local\bin;%PATH%
call .venv\Scripts\activate.bat
python main.py

