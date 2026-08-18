FROM node:22-alpine
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci --omit=dev --no-audit --no-fund
COPY src ./src
COPY public ./public
USER node
ENV NODE_ENV=production
EXPOSE 3000
CMD ["node", "src/entrypoint.mjs"]
