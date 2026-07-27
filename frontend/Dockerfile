# ---------- 构建阶段 ----------
FROM node:20-alpine AS builder

WORKDIR /app

# 先拷贝依赖清单，利用 Docker 层缓存
COPY package.json ./
RUN npm install --no-audit --no-fund --registry=https://registry.npmmirror.com

# 拷贝源码并构建
COPY . .
RUN npm run build

# ---------- 运行阶段 ----------
FROM nginx:alpine

COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=builder /app/dist /usr/share/nginx/html

EXPOSE 80

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -q -O /dev/null http://127.0.0.1/ || exit 1

CMD ["nginx", "-g", "daemon off;"]
