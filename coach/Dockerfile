FROM python:3.13-slim

WORKDIR /app
COPY pyproject.toml ./
COPY src ./src
RUN pip install --no-cache-dir .

# Download the embedding model at build time, so the first coaching summary doesn't wait
# for it. The cache stays writable so that a different EMBEDDING_MODEL can still download.
ARG EMBEDDING_MODEL=minishlab/potion-base-8M
ENV HF_HOME=/app/hf-cache
RUN EMBEDDING_MODEL=$EMBEDDING_MODEL python -m rackwise_coach.notes --download \
    && chown -R nobody /app/hf-cache

USER nobody
EXPOSE 8000
CMD ["uvicorn", "rackwise_coach.main:app", "--host", "0.0.0.0", "--port", "8000"]
