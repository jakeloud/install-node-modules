FROM golang
COPY package.json .
COPY sheit/install.go .
RUN go build install.go
CMD ["./install"]
