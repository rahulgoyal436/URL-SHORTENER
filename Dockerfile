# builder stage
FROM golang:1.24-alpine AS builder
RUN apk add --no-cache git
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /app ./cmd/server

# final stage
FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /app /app
ENV GIN_MODE=release
EXPOSE 8080
ENTRYPOINT ["/app"]
