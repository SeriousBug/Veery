# syntax=docker/dockerfile:1

# --- Stage 1: build the SPA ---
FROM node:26-alpine AS web
WORKDIR /app/web
# Node 26 no longer bundles corepack; install the pinned pnpm directly.
RUN npm install -g pnpm@10.20.0
# Install deps first for layer caching.
COPY web/package.json web/pnpm-lock.yaml* web/pnpm-workspace.yaml* ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
# generated.ts is committed; build produces web/dist.
RUN pnpm build

# --- Stage 2: build the static Go binary embedding the SPA ---
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Replace the committed dist placeholder with the freshly built SPA.
RUN rm -rf web/dist
COPY --from=web /app/web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /veery ./cmd/veery
# Create the data dir so it can be owned by the nonroot runtime user; a mounted
# named volume inherits this ownership.
RUN mkdir -p /data

# --- Stage 3: tiny runtime ---
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /veery /veery
COPY --from=build --chown=65532:65532 /data /data
VOLUME ["/data"]
EXPOSE 8080
ENV VEERY_DB=/data/veery.db VEERY_ADDR=:8080
ENTRYPOINT ["/veery"]
