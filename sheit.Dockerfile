FROM python
COPY sheit/install.py package.json .
CMD ["python", "install.py"]
