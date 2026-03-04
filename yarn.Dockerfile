FROM node:slim
COPY package.json .
RUN yarn
