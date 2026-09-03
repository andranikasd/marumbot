# Build and run Marum. Distroless, static, non-root, no shell in the final image.
FROM node:22-alpine AS assets
WORKDIR /src/deploy/cloudflare
COPY deploy/cloudflare/package*.json ./
RUN npm ci
COPY deploy/cloudflare/build-assets.mjs ./
COPY internal/adapter/in/miniapp/web /src/internal/adapter/in/miniapp/web
RUN npm run build:assets

FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=assets /src/deploy/cloudflare/dist/assets ./internal/adapter/in/miniapp/web
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath \
      -ldflags="-s -w -X main.version=${VERSION}" \
      -o /out/marum ./cmd/marum

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/marum /marum
EXPOSE 8080 8081
USER nonroot:nonroot
ENTRYPOINT ["/marum"]
