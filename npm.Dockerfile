FROM node:slim
COPY package.json .
CMD ["npm", "i"]
