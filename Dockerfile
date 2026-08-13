# syntax=docker/dockerfile:1
FROM golang:1.26.5-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/tdx-api ./cmd/api

FROM alpine:3.23
RUN apk add --no-cache ca-certificates wget && addgroup -S app && adduser -S -G app app
WORKDIR /app
COPY --from=builder /out/tdx-api /app/tdx-api
COPY --from=builder /src/docs /app/docs
USER app
EXPOSE 8080
ENTRYPOINT ["/app/tdx-api"]
