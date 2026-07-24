FROM golang:1.21-alpine AS build
WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download || true
COPY . .
RUN go build -o orchestrator ./cmd/api

FROM alpine:3.19
WORKDIR /app
COPY --from=build /app/orchestrator .
EXPOSE 8080
CMD ["./orchestrator"]
