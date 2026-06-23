FROM golang:1.25-alpine AS builder

RUN apk add --no-cache gcc musl-dev

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 go build -tags sqlite_fts5 -o irollhub .

FROM alpine:3.21

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=builder /app/irollhub .
COPY --from=builder /app/config.yaml .

EXPOSE 8080

VOLUME ["/app/data"]

CMD ["./irollhub"]
