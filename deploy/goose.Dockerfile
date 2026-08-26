# Migration runner. Built once so `make migrate` is fast and offline-capable.
FROM golang:1.25-alpine AS build
RUN go install github.com/pressly/goose/v3/cmd/goose@v3.24.1

FROM alpine:3.24
COPY --from=build /go/bin/goose /usr/local/bin/goose
WORKDIR /migrations
ENTRYPOINT ["goose"]
