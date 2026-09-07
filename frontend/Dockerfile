# dosedu.kz — combined frontend build image.
# Builds all 5 Vue apps and bakes their static output into an nginx
# image, so `docker compose up` needs no separate Node toolchain on
# the host. Nginx config is added separately via the compose volume
# mount (see docker-compose.local.yml).

# ---- one build stage per app ----
FROM node:20-alpine AS build-landing
WORKDIR /src
COPY landing/package.json ./
RUN npm install --no-audit --no-fund
COPY landing/ ./
RUN npm run build

FROM node:20-alpine AS build-s-admin
WORKDIR /src
COPY s-admin/package.json ./
RUN npm install --no-audit --no-fund
COPY s-admin/ ./
RUN npm run build

FROM node:20-alpine AS build-director
WORKDIR /src
COPY director/package.json ./
RUN npm install --no-audit --no-fund
COPY director/ ./
RUN npm run build

FROM node:20-alpine AS build-teacher
WORKDIR /src
COPY teacher/package.json ./
RUN npm install --no-audit --no-fund
COPY teacher/ ./
RUN npm run build

FROM node:20-alpine AS build-app
WORKDIR /src
COPY app/package.json ./
RUN npm install --no-audit --no-fund
COPY app/ ./
RUN npm run build

# ---- final image: nginx + all 5 static builds ----
FROM nginx:1.27-alpine

COPY --from=build-landing  /src/dist /usr/share/nginx/html/landing
COPY --from=build-s-admin  /src/dist /usr/share/nginx/html/s-admin
COPY --from=build-director /src/dist /usr/share/nginx/html/director
COPY --from=build-teacher  /src/dist /usr/share/nginx/html/teacher
COPY --from=build-app      /src/dist /usr/share/nginx/html/app

# Nginx config itself comes from a volume mount in docker-compose.local.yml
# (../deploy/nginx/local), so it can be edited without rebuilding the image.
