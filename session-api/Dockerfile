# Default: AWS Public ECR mirror of Docker Official (avoids flaky registry-1.docker.io). Override: --build-arg GOLANG_IMAGE=golang:1.24-alpine
ARG GOLANG_IMAGE=public.ecr.aws/docker/library/golang:1.24-alpine
FROM ${GOLANG_IMAGE} AS builder

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
