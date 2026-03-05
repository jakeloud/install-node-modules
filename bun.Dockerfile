FROM oven/bun
COPY package.json .
CMD ["bun", "i"]
