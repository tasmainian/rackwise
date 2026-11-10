# Runs the Vite dev server, which also proxies API calls to the other services.
# A production build gets its own image when the app is deployed in Phase 8.
FROM node:22-alpine

WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN chown -R node:node /app

USER node
EXPOSE 5173
CMD ["npm", "run", "dev", "--", "--host", "0.0.0.0"]
