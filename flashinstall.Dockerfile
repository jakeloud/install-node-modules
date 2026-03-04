FROM node:slim
RUN npm i -g @flash-install/cli
COPY package.json .
RUN flash install
