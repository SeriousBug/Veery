# syntax=docker/dockerfile:1

# --- Stage 1: build the SPA ---
FROM node:26-alpine AS web
WORKDIR /app/web
RUN corepack enable
# Install deps first for layer caching.
COPY web/package.json web/pnpm-lock.yaml* web/pnpm-workspace.yaml* ./
RUN corepack pnpm install --frozen-lockfile
COPY web/ ./
# generated.ts is committed; build produces web/dist.
RUN corepack pnpm build

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

# --- Stage 3: tiny runtime ---
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /veery /veery
VOLUME ["/data"]
EXPOSE 8080
ENV VEERY_DB=/data/veery.db VEERY_ADDR=:8080
ENTRYPOINT ["/veery"]
