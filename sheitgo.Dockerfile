FROM golang AS build
WORKDIR /
COPY sheit/install.go .
RUN go build install.go

FROM node:slim
RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates && \
    update-ca-certificates && \
    rm -rf /var/lib/apt/lists/*
COPY --from=build /install /install
COPY package.json .
CMD ["./install"]
