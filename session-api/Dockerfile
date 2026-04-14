FROM golang:1.24-alpine AS builder

WORKDIR /src

COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/session-api ./cmd/session-api

FROM gcr.io/distroless/static-debian12

COPY --from=builder /out/session-api /session-api

EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/session-api"]
