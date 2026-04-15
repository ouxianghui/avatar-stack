# Default: AWS Public ECR mirror of Docker Official (avoids flaky registry-1.docker.io). Override: --build-arg PYTHON_IMAGE=python:3.11-slim
ARG PYTHON_IMAGE=public.ecr.aws/docker/library/python:3.11-slim
FROM ${PYTHON_IMAGE}

WORKDIR /app

RUN apt-get update \
    && apt-get install -y --no-install-recommends ffmpeg \
    && rm -rf /var/lib/apt/lists/*

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY app /app/app

CMD ["python", "-m", "app.worker"]
