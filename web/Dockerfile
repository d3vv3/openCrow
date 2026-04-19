# syntax=docker/dockerfile:1

FROM node:22-alpine AS deps
WORKDIR /app

COPY package.json ./
RUN npm install --no-audit --no-fund

FROM node:22-alpine AS builder-base
WORKDIR /app
ENV NEXT_TELEMETRY_DISABLED=1

COPY --from=deps /app/node_modules ./node_modules
COPY . .

# Optional build-time public API base URL for client bundle
ARG NEXT_PUBLIC_API_URL
ARG NEXT_PUBLIC_API_BASE_URL
ARG NEXT_PUBLIC_APP_URL
ENV NEXT_PUBLIC_API_URL=${NEXT_PUBLIC_API_URL}
ENV NEXT_PUBLIC_API_BASE_URL=${NEXT_PUBLIC_API_BASE_URL}
ENV NEXT_PUBLIC_APP_URL=${NEXT_PUBLIC_APP_URL}

FROM builder-base AS builder-dev
RUN npm run build

FROM builder-base AS builder-prod
RUN npm run build

FROM node:22-alpine AS runtime-base
WORKDIR /app

ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1
ENV HOSTNAME=0.0.0.0
ENV PORT=3000

FROM runtime-base AS runtime-dev
WORKDIR /app
ENV NODE_ENV=development
COPY package.json ./
RUN npm install --no-audit --no-fund
EXPOSE 3000
CMD ["npm", "run", "dev", "--", "--hostname", "0.0.0.0", "--port", "3000"]

FROM runtime-base AS runtime-prod

# Standalone output from Next.js
COPY --from=builder-prod /app/.next/standalone ./
COPY --from=builder-prod /app/.next/static ./.next/static
COPY --from=builder-prod /app/public ./public

USER node
EXPOSE 3000

CMD ["node", "server.js"]
