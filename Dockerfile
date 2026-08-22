FROM golang:1.27-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o myblog-dispatch .

FROM alpine:3.22
RUN apk --no-cache add ca-certificates tzdata && \
    adduser -D -u 1000 appuser
WORKDIR /app
COPY --from=builder /app/myblog-dispatch .
USER appuser
EXPOSE 8080
CMD ["./myblog-dispatch"]
