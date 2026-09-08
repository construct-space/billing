# Stage 1: Build Go binary. The billing service is API-only; the customer
# billing UI is served through the portal gateway.
FROM golang:1.26-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o billing .

# Stage 2: Runtime
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=build /app/billing .
ENV PORT=80
EXPOSE 80
CMD ["./billing"]
