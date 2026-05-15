# partner-api Dockerfile (W0 scaffold; 多阶段构建).
FROM docker.io/library/golang:1.22-alpine AS builder
WORKDIR /src
ENV CGO_ENABLED=0 GOFLAGS=-trimpath
COPY go.mod ./
RUN go mod download || true
COPY . .
RUN go build -o /out/partner-api ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/partner-api /usr/local/bin/partner-api
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/partner-api"]
