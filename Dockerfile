# Build stage
FROM node:20-alpine AS frontend
WORKDIR /app
COPY package*.json ./
RUN npm install
COPY . .
RUN npx tailwindcss -i ./assets/app.css -o ./public/app.css

# Build Go app
FROM golang:1.24-alpine AS builder
WORKDIR /app

# Install dependencies
RUN apk add --no-cache gcc musl-dev git

# Install templ
RUN go install github.com/a-h/templ/cmd/templ@latest

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .
COPY --from=frontend /app/public/app.css ./public/app.css

# Generate templ files
RUN templ generate

# Build binary
RUN CGO_ENABLED=1 go build -ldflags="-s -w" -o holmes ./app

# Final stage
FROM alpine:latest
WORKDIR /app

# Install ca-certificates for HTTPS
RUN apk --no-cache add ca-certificates

# Copy binary and assets
COPY --from=builder /app/holmes .
COPY --from=builder /app/public ./public

# Expose port
EXPOSE 8080

# Run
CMD ["./holmes"]
