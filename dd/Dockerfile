#ARG BUILD_FROM
#ARG TEMPIO_VERSION
#ARG BUILD_ARCH
# Stage 1: Build the Go binary
FROM golang:1.20 AS builder

# Set the working directory inside the container
WORKDIR /app

# Copy Go modules manifests and download dependencies
COPY ../go.mod ../go.sum ./
RUN go mod download

# Copy the source code
COPY ../. .

# Build the Go binary
RUN GOOS=linux GOARCH=amd64  CGO_ENABLED=0 go ./bin/action/main.go -o action
RUN GOOS=linux GOARCH=amd64  CGO_ENABLED=0 go ./bin/haus/main.go -o haus
RUN GOOS=linux GOARCH=amd64  CGO_ENABLED=0 go ./bin/register/main.go -o register

# Stage 2: Create a lightweight runtime container

# https://developers.home-assistant.io/docs/add-ons/configuration#add-on-dockerfile

#FROM $BUILD_FROM
#RUN \
#    curl -sSLf -o /usr/bin/tempio \
#    "https://github.com/home-assistant/tempio/releases/download/${TEMPIO_VERSION}/tempio_${BUILD_ARCH}"
#
## Copy root filesystem
#COPY rootfs /