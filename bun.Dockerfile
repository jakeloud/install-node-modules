FROM oven/bun
COPY package.json .
RUN bun i
