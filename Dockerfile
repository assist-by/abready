FROM golang:1.22-alpine

RUN apk add curl

WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download all dependencies
RUN go mod download

# Copy the source from the current directory to the working Directory inside the container
COPY . .

# Build the Go app
RUN go build -o main .

EXPOSE 8500

# Command to run the executable
CMD ["./main"]