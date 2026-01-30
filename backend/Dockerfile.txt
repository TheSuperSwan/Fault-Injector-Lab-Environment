
FROM registry.access.redhat.com/ubi10/go-toolset:10.0 AS builder

WORKDIR /workspace

COPY guestbook-backend/go.mod guestbook-backend/go.sum guestbook-backend/main.go ./
RUN go mod download

COPY guestbook-backend/ .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o guestbook-api .

FROM registry.access.redhat.com/ubi10-minimal:10.0

WORKDIR /app
COPY --from=builder /workspace/guestbook-api /app/guestbook-api

CMD ["/app/guestbook-api"]
