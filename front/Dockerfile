# ── Stage 1 : Dépendances ────────────────────────────────────
FROM node:20-alpine3.22 AS deps

WORKDIR /app

COPY package.json package-lock.json ./
RUN npm ci --ignore-scripts

# ── Stage 2 : Dev ────────────────────────────────────────────
FROM node:20-alpine3.22 AS dev

WORKDIR /app

COPY --from=deps /app/node_modules ./node_modules
COPY . .

EXPOSE 5173

CMD ["npm", "run", "dev"]

# ── Stage 3 : Build ──────────────────────────────────────────
FROM node:20-alpine3.22 AS builder

WORKDIR /app

COPY --from=deps /app/node_modules ./node_modules
COPY . .

ARG VITE_API_URL
ENV VITE_API_URL=${VITE_API_URL}

RUN npm run build

# ── Stage 3 : Prod ───────────────────────────────────────────
FROM nginx:1.29.5-alpine3.23 AS prod

RUN rm /etc/nginx/conf.d/default.conf

COPY --from=builder /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 5173