# Stage 1: Build
FROM docker.io/oven/bun:1.1 AS builder

WORKDIR /app

COPY package.json bun.lockb* ./
RUN bun install

COPY . .
RUN bun run build

# Stage 2: Production image (served by nginx)
FROM docker.io/library/nginx:1.27-alpine

COPY --from=builder /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 80

CMD ["nginx", "-g", "daemon off;"]
