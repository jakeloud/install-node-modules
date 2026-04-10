FROM golang AS build
WORKDIR /
COPY sheit/install.go .
RUN go build install.go

FROM node:slim
COPY --from=build /install /install
COPY package.json .
CMD ["./install"]
