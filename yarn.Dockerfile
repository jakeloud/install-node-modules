FROM node:slim
COPY package.json .
CMD ["yarn"]
