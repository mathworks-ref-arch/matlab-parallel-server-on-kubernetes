# Copyright 2024 The MathWorks, Inc.

# Stage 1: Build the controller executable
FROM golang:1.21.6 as builder
WORKDIR /app
COPY src/ /app
RUN go version
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o controller /app/cmd/main.go

# Stage 2: Build the controller image
FROM scratch
LABEL maintainer="The MathWorks"
COPY --from=builder /app/controller /controller

ENTRYPOINT ["./controller"]
