FROM golang:1.23-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN set -eux; \
    mkdir -p /out; \
    for service in ingestion query-api persistence-worker analytics-worker webhook-worker; do \
      CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w' -o "/out/${service}" "./cmd/${service}"; \
    done

FROM alpine:3.21

RUN addgroup -S -g 65532 pulse && adduser -S -D -H -u 65532 -G pulse pulse \
    && apk add --no-cache ca-certificates
COPY --from=build /out /usr/local/bin/
COPY deployments/docker/entrypoint.sh /usr/local/bin/pulse-entrypoint
RUN chmod 0555 /usr/local/bin/pulse-entrypoint /usr/local/bin/ingestion \
    /usr/local/bin/query-api /usr/local/bin/persistence-worker \
    /usr/local/bin/analytics-worker /usr/local/bin/webhook-worker
USER 65532:65532
ENV PULSE_HTTP_ADDR=:8080
ENTRYPOINT ["/usr/local/bin/pulse-entrypoint"]
