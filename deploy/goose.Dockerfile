# Migration runner. Built once so `make migrate` is fast and offline-capable.
FROM golang:1.27-alpine AS build
RUN go install github.com/pressly/goose/v3/cmd/goose@v3.24.1

FROM alpine:3.24
# Without CA roots goose cannot verify a TLS database (sslmode=require at a
# managed origin); local sslmode=disable hides the gap until the one run that
# matters.
RUN apk add --no-cache ca-certificates
COPY --from=build /go/bin/goose /usr/local/bin/goose
WORKDIR /migrations
ENTRYPOINT ["goose"]
