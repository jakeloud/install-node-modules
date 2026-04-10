FROM golang
COPY package.json .
COPY sheit/install.go .
CMD ["go", "run", "install.go"]
