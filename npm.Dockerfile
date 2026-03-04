FROM node:slim
COPY package.json .
RUN npm i
