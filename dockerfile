FROM debian:stable-slim

RUN groupadd -r appgroup && \
    useradd -r -g appgroup --no-create-home -s /bin/false appuser

WORKDIR /app

COPY --chown=appuser:appgroup ./build/bin/main .

USER appuser

ENTRYPOINT ["./main"]
