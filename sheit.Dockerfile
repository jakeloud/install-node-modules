FROM node:slim
ENV PYTHONUNBUFFERED=1
RUN apt-get update && \
    apt-get install -y --no-install-recommends python3 python3-pip curl && \
    rm -rf /var/lib/apt/lists/*

COPY package.json .
COPY sheit/install.py .
CMD ["python", "install.py", "-c", "1"]
