# Stage 1: Build the Go application and cache dependencies
FROM docker.io/golang:1.21 as builder

WORKDIR /build

# Copy and download dependencies separately to leverage caching
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source code
COPY *.go ./

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -o cursed-status-page .

# Stage 2: Create a minimal runtime image
FROM alpine:latest

WORKDIR /

# Copy the compiled application from the previous stage
COPY --from=builder /build/cursed-status-page ./

# Copy other necessary files
COPY static/. ./static/
COPY templates/. ./templates/

# Install any necessary runtime dependencies
RUN apk add --no-cache tzdata

# Define the entry point
ENTRYPOINT ["./cursed-status-page"]

