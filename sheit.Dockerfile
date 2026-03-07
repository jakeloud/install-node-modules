FROM python:slim
ENV PYTHONUNBUFFERED=1
RUN apt-get update && apt-get install -y curl && rm -rf /var/lib/apt/lists/*
COPY package.json .
COPY sheit/install.py .
CMD ["python", "install.py"]
