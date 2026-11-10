FROM golang:1.23-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/noitu ./cmd/api

FROM alpine:3.21

RUN apk add --no-cache ca-certificates wget
RUN addgroup -S noitu && adduser -S -G noitu noitu
COPY --from=build /out/noitu /usr/local/bin/noitu

USER noitu
EXPOSE 8080
ENV ADDR=:8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD wget -q -O /dev/null http://127.0.0.1:8080/healthz || exit 1
ENTRYPOINT ["/usr/local/bin/noitu"]
