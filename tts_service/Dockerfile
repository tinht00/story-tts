FROM python:3.13-slim-bookworm

WORKDIR /app

COPY tts_service/requirements.txt ./tts_service/requirements.txt
RUN python -m pip install --no-cache-dir --upgrade pip \
    && python -m pip install --no-cache-dir -r ./tts_service/requirements.txt

COPY tts_service ./tts_service

EXPOSE 8010

CMD ["python", "-m", "uvicorn", "tts_service.app:app", "--host", "0.0.0.0", "--port", "8010"]
