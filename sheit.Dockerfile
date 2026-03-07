FROM python:slim
COPY sheit/install.py package.json .
CMD ["python", "install.py"]
