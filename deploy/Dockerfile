# Build and run Marum. Distroless, static, non-root, no shell in the final image.
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath \
      -ldflags="-s -w -X main.version=${VERSION}" \
      -o /out/marum ./cmd/marum

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/marum /marum
EXPOSE 8080 8081
USER nonroot:nonroot
ENTRYPOINT ["/marum"]
