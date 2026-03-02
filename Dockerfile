FROM --platform=$BUILDPLATFORM golang:1.23-alpine AS builder

ARG TARGETARCH=arm64
ARG TARGETOS=linux

WORKDIR /app

RUN apk add --no-cache ca-certificates

COPY go.mod go.sum* ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o /searchmux ./cmd/searchmux

FROM --platform=linux/arm64 alpine:3.23

WORKDIR /app

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /searchmux /app/searchmux

RUN addgroup -g 1001 -S appgroup && \
    adduser -u 1001 -S appuser -G appgroup && \
    chown -R appuser:appgroup /app

USER appuser

EXPOSE 8080

ENV PORT=8080

CMD ["/app/searchmux", "--config", "/etc/searchmux/config.yaml"]
